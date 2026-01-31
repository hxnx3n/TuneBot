package commands

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	shared "github.com/hxnx/tunebot/internal/features/shared"
	"github.com/hxnx/tunebot/internal/music"
)

const defaultQueueLimit = int64(10)

func Queue(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if i.GuildID == "" {
		shared.RespondEphemeral(s, i, "이 명령어는 서버에서만 사용할 수 있습니다.")
		return
	}

	data := i.ApplicationCommandData()
	limit := shared.GetOptionInt64(data.Options, "limit")
	if limit <= 0 {
		limit = defaultQueueLimit
	}

	store := music.NewQueueStoreFromDefault()
	if store == nil {
		shared.RespondEphemeral(s, i, "대기열을 조회할 수 없습니다.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	items, err := store.List(ctx, i.GuildID, limit)
	if err != nil {
		log.Printf("queue error: %v", err)
		shared.RespondEphemeral(s, i, "대기열을 불러오지 못했습니다.")
		return
	}

	if len(items) == 0 {
		shared.RespondEphemeral(s, i, "대기열이 비어 있습니다.")
		return
	}

	lines := make([]string, 0, len(items))
	for idx, item := range items {
		lines = append(lines, fmt.Sprintf("%d. [%s](%s)", idx+1, item.Track.Title, item.Track.URL))
	}

	shared.RespondEphemeral(s, i, strings.Join(lines, "\n"))
}
