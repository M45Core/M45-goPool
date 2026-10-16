package main

import (
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestWorkerHasRecentShares(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	threshold := 5 * time.Minute
	stale := WorkerView{LastShare: now.Add(-threshold), RollingHashrate: 1e12}
	recent := WorkerView{LastShare: now.Add(-time.Minute)}
	for _, tt := range []struct {
		name  string
		views []WorkerView
		want  bool
	}{
		{"disconnected", nil, false},
		{"recent share", []WorkerView{recent}, true},
		{"stale positive hashrate", []WorkerView{stale}, false},
		{"new connection grace", []WorkerView{{ConnectedAt: now}}, true},
		{"never submitted", []WorkerView{{ConnectedAt: now.Add(-threshold)}}, false},
		{"missing timestamps", []WorkerView{{}}, false},
		{"one active connection", []WorkerView{stale, recent}, true},
		{"all connections stalled", []WorkerView{stale, stale}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := workerHasRecentShares(tt.views, now, threshold); got != tt.want {
				t.Fatalf("active = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiscordNotifierCheckUserActivity(t *testing.T) {
	for _, mode := range []string{"disconnected", "stalled", "never submitted", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			store, err := newWorkerListStore(t.TempDir() + "/workers.sqlite")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			const worker = "wallet.worker"
			link := discordLink{UserID: "user", DiscordUserID: "discord-user"}
			if err := store.Add(link.UserID, worker); err != nil {
				t.Fatal(err)
			}
			hash := workerNameHash(worker)
			now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			threshold := 5 * time.Minute
			s := &StatusServer{workerLists: store, workerRegistry: newWorkerConnectionRegistry()}
			cfg := defaultConfig()
			cfg.DiscordWorkerNotifyThresholdSeconds = int(threshold / time.Second)
			s.UpdateConfig(cfg)
			n := &discordNotifier{s: s, dg: &discordgo.Session{}, notifyChannelID: "channel"}
			mc := &MinerConn{conn: nopConn{}, connectedAt: now, connectionSeq: 1}
			mc.updateWorker(worker)
			mc.stats.LastShare = now
			s.workerRegistry.register(hash, "", mc)
			if mode == "never submitted" {
				mc.stats.LastShare = time.Time{}
			}
			if mode == "disabled" {
				if err := store.SetSavedWorkerNotifyEnabled(link.UserID, hash, false, now); err != nil {
					t.Fatal(err)
				}
			}
			n.checkUser(link, now)
			if len(n.pingQueue) != 0 {
				t.Fatal("alert on first observation")
			}
			inactiveAt := now.Add(threshold)
			if mode == "disconnected" {
				s.workerRegistry.unregister(hash, "", mc)
			}
			n.checkUser(link, inactiveAt)
			if len(n.pingQueue) != 0 {
				t.Fatal("alert before sustained inactivity")
			}
			alertAt := inactiveAt.Add(threshold)
			n.checkUser(link, alertAt.Add(-time.Second))
			if len(n.pingQueue) != 0 {
				t.Fatal("alert before threshold")
			}
			n.checkUser(link, alertAt)
			if mode == "disabled" {
				if len(n.pingQueue) != 0 {
					t.Fatal("alert for disabled worker")
				}
				return
			}
			if len(n.pingQueue) != 1 {
				t.Fatalf("queued messages = %d, want 1", len(n.pingQueue))
			}
			content, mentions, _ := renderQueuedMessage(n.pingQueue[0])
			want := "Not mining"
			if mode == "disconnected" {
				want = "Offline >5m"
			}
			if !strings.Contains(content, want) || len(mentions) != 1 || mentions[0] != link.DiscordUserID {
				t.Fatalf("unexpected alert: %q, mentions %v", content, mentions)
			}
			n.pingQueue = nil
			n.checkUser(link, alertAt.Add(time.Minute))
			if len(n.pingQueue) != 0 {
				t.Fatal("duplicate outage alert")
			}
			// Resume shares and require sustained recovery before notifying.
			recovered := alertAt.Add(2 * time.Minute)
			if mode == "disconnected" {
				s.workerRegistry.register(hash, "", mc)
			}
			mc.stats.LastShare = recovered
			n.checkUser(link, recovered)
			mc.stats.LastShare = recovered.Add(threshold - time.Second)
			n.checkUser(link, mc.stats.LastShare)
			if len(n.pingQueue) != 0 {
				t.Fatal("premature recovery alert")
			}
			mc.stats.LastShare = recovered.Add(threshold)
			n.checkUser(link, mc.stats.LastShare)
			if len(n.pingQueue) != 1 {
				t.Fatal("missing recovery alert")
			}
			content, _, _ = renderQueuedMessage(n.pingQueue[0])
			if !strings.Contains(content, "Back online/mining (5m+)") {
				t.Fatalf("unexpected recovery: %q", content)
			}
		})
	}
}

func TestDiscordNotifierAlreadyOfflineAfterResetStaysQuiet(t *testing.T) {
	n := &discordNotifier{}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	n.updateWorkerStates("user", map[string]bool{"worker": true}, now)
	n.resetAllNotificationState(now.Add(time.Minute))
	resumed := n.startChecksAt
	offline, online := n.updateWorkerStates("user", map[string]bool{"worker": false}, resumed)
	if len(offline) != 0 || len(online) != 0 {
		t.Fatal("immediate alert after reset")
	}
	offline, online = n.updateWorkerStates("user", map[string]bool{"worker": false}, resumed.Add(n.workerNotifyThreshold()))
	if len(offline) != 0 || len(online) != 0 {
		t.Fatal("alert for worker already offline after reset")
	}
	// Once activity resumes, subsequent outages must still alert.
	activeAt := resumed.Add(2 * n.workerNotifyThreshold())
	n.updateWorkerStates("user", map[string]bool{"worker": true}, activeAt)
	inactiveAt := activeAt.Add(n.workerNotifyThreshold())
	n.updateWorkerStates("user", map[string]bool{"worker": false}, inactiveAt)
	offline, online = n.updateWorkerStates("user", map[string]bool{"worker": false}, inactiveAt.Add(n.workerNotifyThreshold()))
	if len(offline) != 1 || offline[0] != "worker" || len(online) != 0 {
		t.Fatal("missing alert for new outage after reset")
	}
}

func TestDiscordNotifierCheckUserPreservesStateOnStoreError(t *testing.T) {
	store, err := newWorkerListStore(t.TempDir() + "/workers.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	n := &discordNotifier{s: &StatusServer{workerLists: store}, dg: &discordgo.Session{}, notifyChannelID: "channel"}
	now := time.Now()
	n.updateWorkerStates("user", map[string]bool{"worker": false}, now)
	n.checkUser(discordLink{UserID: "user", DiscordUserID: "discord-user"}, now.Add(time.Minute))
	if len(n.statusByUser["user"]) != 1 {
		t.Fatal("store error erased outage timer")
	}
}
