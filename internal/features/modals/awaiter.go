package modals

import (
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type ModalResponse struct {
	Interaction *discordgo.InteractionCreate
	Data        discordgo.ModalSubmitInteractionData
	Err         error
}

type ComponentResponse struct {
	Interaction *discordgo.InteractionCreate
	Data        discordgo.MessageComponentInteractionData
	Err         error
}

type Awaiter struct {
	pending map[string]chan *discordgo.InteractionCreate
	mu      sync.RWMutex
}

var DefaultAwaiter = NewAwaiter()

func NewAwaiter() *Awaiter {
	return &Awaiter{
		pending: make(map[string]chan *discordgo.InteractionCreate),
	}
}

func (a *Awaiter) ShowAndAwaitModal(
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	modal *discordgo.InteractionResponseData,
	timeout time.Duration,
) (*ModalResponse, error) {
	userID := getUserID(i)
	if userID == "" {
		return nil, fmt.Errorf("missing user id for interaction")
	}

	key := modal.CustomID + ":" + userID
	ch := make(chan *discordgo.InteractionCreate, 1)

	a.mu.Lock()
	a.pending[key] = ch
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		delete(a.pending, key)
		a.mu.Unlock()
	}()

	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: modal,
	})
	if err != nil {
		return nil, err
	}

	select {
	case submission := <-ch:
		if submission.Type != discordgo.InteractionModalSubmit {
			return nil, fmt.Errorf("unexpected interaction type: %v", submission.Type)
		}
		return &ModalResponse{
			Interaction: submission,
			Data:        submission.ModalSubmitData(),
		}, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("modal timed out")
	}
}

func (a *Awaiter) AwaitComponent(
	i *discordgo.InteractionCreate,
	customID string,
	timeout time.Duration,
) (*ComponentResponse, error) {
	userID := getUserID(i)
	if userID == "" {
		return nil, fmt.Errorf("missing user id for interaction")
	}

	key := customID + ":" + userID
	ch := make(chan *discordgo.InteractionCreate, 1)

	a.mu.Lock()
	a.pending[key] = ch
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		delete(a.pending, key)
		a.mu.Unlock()
	}()

	select {
	case interaction := <-ch:
		if interaction.Type != discordgo.InteractionMessageComponent {
			return nil, fmt.Errorf("unexpected interaction type: %v", interaction.Type)
		}
		return &ComponentResponse{
			Interaction: interaction,
			Data:        interaction.MessageComponentData(),
		}, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("component timed out")
	}
}

func (a *Awaiter) HandleInteraction(i *discordgo.InteractionCreate) bool {
	customID, ok := extractCustomID(i)
	if !ok || customID == "" {
		return false
	}

	userID := getUserID(i)
	if userID == "" {
		return false
	}

	key := customID + ":" + userID

	a.mu.RLock()
	ch, exists := a.pending[key]
	a.mu.RUnlock()

	if !exists {
		return false
	}

	select {
	case ch <- i:
		return true
	default:
		return false
	}
}

func extractCustomID(i *discordgo.InteractionCreate) (string, bool) {
	switch i.Type {
	case discordgo.InteractionMessageComponent:
		data := i.MessageComponentData()
		return data.CustomID, true
	case discordgo.InteractionModalSubmit:
		data := i.ModalSubmitData()
		return data.CustomID, true
	default:
		return "", false
	}
}

func getUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}
