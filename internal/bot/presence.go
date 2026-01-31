package bot

import (
	"fmt"
	"log"
	"time"
)

const presenceUpdateInterval = 60 * time.Second

func (b *Bot) startPresenceUpdater() {
	if b.presenceStop != nil {
		return
	}
	b.presenceStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(presenceUpdateInterval)
		defer ticker.Stop()

		b.updatePresence()
		for {
			select {
			case <-b.presenceStop:
				return
			case <-ticker.C:
				b.updatePresence()
			}
		}
	}()
}

func (b *Bot) stopPresenceUpdater() {
	if b.presenceStop == nil {
		return
	}
	close(b.presenceStop)
	b.presenceStop = nil
}

func (b *Bot) updatePresence() {
	for _, s := range b.sessions {
		guildCount := 0
		if s.State != nil {
			guildCount = len(s.State.Guilds)
		}

		shardNumber := max(1, s.ShardID+1)

		status := fmt.Sprintf("#%d샤드 / %d개 서버 참가중", shardNumber, guildCount)
		if err := s.UpdateGameStatus(0, status); err != nil {
			log.Printf("failed to update presence: %v", err)
		}
	}
}
