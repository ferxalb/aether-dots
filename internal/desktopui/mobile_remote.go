package desktopui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"aether-dots/internal/engine"
	"aether-dots/internal/integrations/remote"
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) initializeRemoteBridge() {
	a.remoteCredentialStore = remote.NewKeychainStore()
	receipts, err := remote.OpenReceiptStore(a.dataDir)
	if err != nil {
		a.remoteStatus = "Unavailable"
		a.remoteError = err.Error()
		return
	}
	a.remoteReceipts = receipts

	client, err := remote.Configured()
	if err != nil {
		a.remoteStatus = "Unavailable in this build"
		a.remoteError = "This desktop build does not include the approved Cloudflare relay endpoint."
		return
	}
	a.remoteClient = client
	credential, err := a.remoteCredentialStore.Load(context.Background())
	if err != nil {
		a.remoteStatus = "Disconnected"
		return
	}
	a.remoteDevice = credential
	a.remoteClient = client.WithDevice(credential)
	a.remoteStatus = "Connected"
	a.startRemoteCommandLoop()
}

func (a *app) mobileRemoteCard(c *ui.Context, theme *ui.Theme) {
	status := a.remoteStatus
	if status == "" {
		status = "Checking connection"
	}
	card := ui.Column(c.Key("integration-mobile-card")).FillWidth().MinWidth(0).Gap(13).Padding(17, 18).
		Radius(15).Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
			mark := ui.Box(c).Size(42, 42).Shrink(0).Radius(14).Background(theme.Background).Border(1, theme.Border)
			mark.Center().Children(func() { ui.Text(c, "⌘").FontSize(20).Bold() })
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, "AETHER Dots for iPhone").FontSize(15).Bold()
				ui.Text(c, "Check on your Dots and send them a message from iOS.").FontSize(11).TextColor(theme.TextMuted).FillWidth()
			})
			ui.Text(c, status).FontSize(10).Bold().TextColor(remoteStatusColor(theme, status))
		})

		ui.Text(c, "Pairing lets your signed-in iPhone list Dots, open a conversation when requested, and send messages through this Mac. A message uses the Dot’s saved provider settings and existing specialist or GitHub grants. Chat text and remote requests pass through the AETHER Cloudflare relay and Neon; your local files and provider keys stay on this Mac.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth()

		if a.remoteError != "" {
			ui.Row(c).FillWidth().Gap(8).Padding(10, 11).Radius(10).Background(theme.Danger.Alpha(0.08)).Children(func() {
				ui.Text(c, a.remoteError).FontSize(11).TextColor(theme.Danger).FillWidth()
			})
		}

		if a.remotePairCode != "" {
			ui.Column(c).FillWidth().Gap(8).Padding(12, 14).Radius(11).Background(theme.Background).Border(1, theme.Border).Children(func() {
				ui.Text(c, "Enter this one-time code in AETHER Dots on your iPhone").FontSize(11).Bold()
				ui.Text(c, a.remotePairCode).FontSize(22).Bold().LetterSpacing(1.5)
				ui.Row(c).Wrap().Gap(8).Children(func() {
					if ui.Button(c.Key("mobile-copy-code"), "Copy code").Padding(8, 11).Clicked() {
						c.WriteClipboard(a.remotePairCode)
						a.adt.Success("Code copied", "Enter the one-time code in AETHER Dots on your iPhone.")
					}
					if ui.Button(c.Key("mobile-cancel-pairing"), "Cancel pairing").Padding(8, 11).Clicked() {
						a.cancelRemotePairing()
					}
				})
				ui.Text(c, "The code expires after 10 minutes. Keep it private; AETHER will finish connecting after the iPhone claims it.").FontSize(10).TextColor(theme.TextMuted).FillWidth()
			})
		}

		ui.Divider(c)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
			if a.remoteStatus == "Connected" {
				ui.Text(c, "This Mac is paired with one AETHER account. Disconnecting revokes its relay credential.").FontSize(10).TextColor(theme.TextMuted)
				ui.Spacer(c)
				if ui.Button(c.Key("mobile-disconnect"), "Disconnect iPhone access").Padding(8, 12).Clicked() {
					a.disconnectRemote()
				}
			} else if a.remoteStatus == "Disconnecting" {
				ui.Text(c, "Disconnecting from iPhone…").FontSize(11).TextColor(theme.TextMuted)
			} else if a.remotePairBusy {
				ui.Text(c, "Waiting for the iPhone to claim the code…").FontSize(11).TextColor(theme.TextMuted)
			} else {
				button := ui.PrimaryButton(c.Key("mobile-start-pairing"), "Start pairing").Padding(8, 12).
					Disabled(a.remoteClient == nil)
				if button.Clicked() {
					a.startRemotePairing()
				}
				if a.remoteClient == nil {
					ui.Text(c, "AETHER releases must include the pinned Cloudflare Worker origin before pairing is available.").FontSize(10).TextColor(theme.TextMuted).FillWidth()
				}
			}
		})
	})
}

func remoteStatusColor(theme *ui.Theme, status string) ui.Color {
	switch status {
	case "Connected":
		return theme.Success
	case "Unavailable", "Unavailable in this build", "Connection expired":
		return theme.Danger
	default:
		return theme.TextMuted
	}
}

func (a *app) startRemotePairing() {
	if a.remoteClient == nil || a.remotePairBusy || a.remoteStatus == "Connected" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.remotePairCancel = cancel
	a.remotePairBusy = true
	a.remoteError = ""
	a.remoteStatus = "Starting pairing"
	go a.runRemotePairing(ctx)
}

func (a *app) runRemotePairing(ctx context.Context) {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		name = "AETHER Desktop"
	}
	name = truncateUTF8Bytes(strings.TrimSpace(name), 80)
	startCtx, startCancel := context.WithTimeout(ctx, 12*time.Second)
	pairing, err := a.remoteClient.StartPairing(startCtx, name)
	startCancel()
	if err != nil {
		a.updateRemote(func() {
			a.remotePairBusy = false
			a.remotePairCancel = nil
			a.remoteStatus = "Disconnected"
			a.remoteError = err.Error()
		})
		return
	}
	a.updateRemote(func() {
		a.remotePairCode = pairing.Code
		a.remoteStatus = "Waiting for iPhone"
	})

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = a.remoteClient.CancelPairing(cancelCtx, pairing.Code)
			cancel()
			a.updateRemote(func() {
				a.remotePairBusy = false
				a.remotePairCancel = nil
				a.remotePairCode = ""
				a.remoteStatus = "Disconnected"
			})
			return
		case <-ticker.C:
			checkCtx, checkCancel := context.WithTimeout(ctx, 12*time.Second)
			state, err := a.remoteClient.PairingStatus(checkCtx, pairing.Code)
			checkCancel()
			if err != nil {
				a.updateRemote(func() { a.remoteError = "Waiting for the iPhone. Check the network and keep this window open." })
				continue
			}
			switch state {
			case "pending":
				continue
			case "expired":
				a.updateRemote(func() {
					a.remotePairBusy = false
					a.remotePairCancel = nil
					a.remotePairCode = ""
					a.remoteStatus = "Disconnected"
					a.remoteError = "The one-time pairing code expired. Start pairing again."
				})
				return
			case "claimed":
				confirmCtx, confirmCancel := context.WithTimeout(ctx, 12*time.Second)
				credential, err := a.remoteClient.ConfirmPairing(confirmCtx, pairing.Code)
				confirmCancel()
				if err == nil {
					err = a.remoteCredentialStore.Save(ctx, credential)
				}
				if err != nil {
					if credential.Token != "" {
						cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 8*time.Second)
						_ = a.remoteClient.WithDevice(credential).Disconnect(cleanupCtx)
						cleanupCancel()
					}
					a.updateRemote(func() {
						a.remotePairBusy = false
						a.remotePairCancel = nil
						a.remotePairCode = ""
						a.remoteStatus = "Disconnected"
						a.remoteError = "AETHER couldn't finish saving the secure connection. Try pairing again."
					})
					return
				}
				a.updateRemote(func() {
					a.remoteDevice = credential
					a.remoteClient = a.remoteClient.WithDevice(credential)
					a.remotePairBusy = false
					a.remotePairCancel = nil
					a.remotePairCode = ""
					a.remoteStatus = "Connected"
					a.remoteError = ""
					a.startRemoteCommandLoop()
				})
				return
			default:
				a.updateRemote(func() { a.remoteError = "The pairing state could not be read. Start pairing again if this continues." })
			}
		}
	}
}

func (a *app) cancelRemotePairing() {
	if a.remotePairCancel != nil {
		a.remotePairCancel()
	}
}

func (a *app) disconnectRemote() {
	if a.remoteClient == nil || a.remoteStatus != "Connected" {
		return
	}
	a.cancelRemoteCommandLoop()
	a.remoteStatus = "Disconnecting"
	a.remoteError = ""
	client := a.remoteClient
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		err := client.Disconnect(ctx)
		cancel()
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
			a.updateRemote(func() {
				a.remoteStatus = "Connected"
				a.remoteError = "The relay could not confirm disconnection. Check the network and try again."
				a.startRemoteCommandLoop()
			})
			return
		}
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		deleteErr := a.remoteCredentialStore.Delete(ctx)
		cancel()
		if deleteErr != nil {
			a.updateRemote(func() {
				a.remoteStatus = "Disconnected"
				a.remoteError = "Relay access was revoked, but the saved credential could not be removed from Keychain."
			})
			return
		}
		a.updateRemote(func() {
			a.remoteDevice = remote.DeviceCredential{}
			a.remoteClient = a.remoteClient.WithDevice(remote.DeviceCredential{})
			a.remoteStatus = "Disconnected"
			a.remoteError = ""
		})
	}()
}

func (a *app) startRemoteCommandLoop() {
	if a.remoteClient == nil || a.remoteDevice.Token == "" || a.remoteReceipts == nil {
		return
	}
	a.cancelRemoteCommandLoop()
	ctx, cancel := context.WithCancel(context.Background())
	a.remoteLoopMu.Lock()
	a.remoteLoopCancel = cancel
	a.remoteLoopMu.Unlock()
	client := a.remoteClient
	go a.remoteHeartbeatLoop(ctx, client)
	go a.remoteCommandLoop(ctx, client)
}

func (a *app) cancelRemoteCommandLoop() {
	a.remoteLoopMu.Lock()
	cancel := a.remoteLoopCancel
	a.remoteLoopCancel = nil
	a.remoteLoopMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *app) remoteHeartbeatLoop(ctx context.Context, client *remote.Client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := client.Heartbeat(reqCtx)
			cancel()
			if isRemoteUnauthorized(err) {
				a.forgetRemoteCredential()
				return
			}
		}
	}
}

func (a *app) remoteCommandLoop(ctx context.Context, client *remote.Client) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
			command, err := client.NextCommand(reqCtx)
			cancel()
			if isRemoteUnauthorized(err) {
				a.forgetRemoteCredential()
				return
			}
			if err != nil || command == nil {
				continue
			}
			if a.remoteReceipts.Seen(command.ID) {
				completeCtx, completeCancel := context.WithTimeout(ctx, 12*time.Second)
				_ = client.CompleteCommand(completeCtx, command.ID, "failed", nil)
				completeCancel()
				continue
			}
			if err := a.remoteReceipts.Record(command.ID); err != nil {
				completeCtx, completeCancel := context.WithTimeout(ctx, 12*time.Second)
				_ = client.CompleteCommand(completeCtx, command.ID, "failed", nil)
				completeCancel()
				continue
			}
			result, err := a.executeRemoteCommand(ctx, *command)
			state := "succeeded"
			if err != nil {
				state = "failed"
			}
			completeCtx, completeCancel := context.WithTimeout(context.Background(), 12*time.Second)
			completeErr := client.CompleteCommand(completeCtx, command.ID, state, result)
			completeCancel()
			if completeErr == nil {
				a.updateRemote(func() {
					a.refresh()
					if err != nil {
						a.remoteError = "A remote action could not be completed. Open this Dot on the Mac for details."
					} else {
						a.remoteError = ""
					}
				})
			}
		}
	}
}

func (a *app) forgetRemoteCredential() {
	a.remoteForgetMu.Lock()
	defer a.remoteForgetMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = a.remoteCredentialStore.Delete(ctx)
	cancel()
	a.cancelRemoteCommandLoop()
	a.updateRemote(func() {
		a.remoteDevice = remote.DeviceCredential{}
		if a.remoteClient != nil {
			a.remoteClient = a.remoteClient.WithDevice(remote.DeviceCredential{})
		}
		a.remoteStatus = "Connection expired"
		a.remoteError = "This Mac’s relay access was revoked. Start pairing again to reconnect."
	})
}

func isRemoteUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unauthorized") || strings.Contains(message, "status 401")
}

func (a *app) updateRemote(update func()) {
	win := a.win
	if win == nil || win.IsDestroyed() {
		update()
		return
	}
	win.Update(update)
}

type remoteAction struct {
	Action  string `json:"action"`
	DotID   string `json:"dotId"`
	Message string `json:"message"`
}

func (a *app) executeRemoteCommand(ctx context.Context, command remote.Command) (any, error) {
	var action remoteAction
	if err := json.Unmarshal(command.Payload, &action); err != nil || action.Action != command.Action {
		return nil, errors.New("invalid remote action")
	}
	snapshot := a.db.Snapshot()
	switch action.Action {
	case "get_status":
		active := make([]map[string]any, 0, 8)
		for _, dot := range snapshot.Dots {
			if dot.Status == task.Captured || dot.Status == task.Ready || dot.Status == task.Running || dot.Status == task.WaitingApproval || dot.Status == task.WaitingDevice || dot.Status == task.WaitingInput {
				active = append(active, map[string]any{"id": dot.ID, "name": dot.Name, "status": dot.Status, "updatedAt": dot.UpdatedAt})
				if len(active) == 8 {
					break
				}
			}
		}
		return map[string]any{"dotCount": len(snapshot.Conversations), "active": active}, nil
	case "list_dots":
		dots := make([]map[string]any, 0, min(50, len(snapshot.Conversations)))
		conversations := append([]task.Conversation(nil), snapshot.Conversations...)
		sortConversations(conversations)
		for _, conversation := range conversations[:min(50, len(conversations))] {
			lastMessage := ""
			if count := len(conversation.Messages); count > 0 {
				lastMessage = truncateUTF8Bytes(conversation.Messages[count-1].Content, 64)
			}
			dots = append(dots, map[string]any{
				"id": conversation.ID, "name": conversation.Name, "icon": conversation.Icon,
				"color": conversation.Color, "lastMessage": lastMessage, "updatedAt": conversation.UpdatedAt,
			})
		}
		return map[string]any{"dots": dots}, nil
	case "get_conversation":
		conversation, ok := findConversation(snapshot, action.DotID)
		if !ok {
			return nil, errors.New("Dot not found")
		}
		messages := make([]map[string]any, 0, 8)
		start := max(0, len(conversation.Messages)-8)
		for _, message := range conversation.Messages[start:] {
			messages = append(messages, map[string]any{
				"id": message.ID, "role": message.Role,
				"content": truncateUTF8Bytes(message.Content, 1024), "createdAt": message.CreatedAt,
			})
		}
		return map[string]any{"dot": map[string]any{
			"id": conversation.ID, "name": conversation.Name, "icon": conversation.Icon,
			"color": conversation.Color, "messages": messages,
		}}, nil
	case "send_message":
		conversation, ok := findConversation(snapshot, action.DotID)
		if !ok {
			return nil, errors.New("Dot not found")
		}
		dot, _, err := a.db.CreateConversationRun(conversation.ID, action.Message)
		if err != nil {
			return nil, err
		}
		prefs := defaultPreferences()
		if current := a.routinePreferences.Load(); current != nil {
			prefs = *current
		}
		generator := generatorForConversationRun(prefs, a.db, dot.ID, a.githubService, func() {})
		if err := engine.RunWithProgress(ctx, a.db, dot.ID, generator, func(string) {}, func() {}); err != nil {
			return nil, errors.New("Dot reply failed")
		}
		updated, _ := findConversation(a.db.Snapshot(), conversation.ID)
		reply := ""
		for index := len(updated.Messages) - 1; index >= 0; index-- {
			message := updated.Messages[index]
			if message.DotID == dot.ID && message.Role == "assistant" {
				reply = truncateUTF8Bytes(message.Content, 4096)
				break
			}
		}
		a.updateRemote(a.refresh)
		return map[string]any{"dotId": conversation.ID, "status": "done", "reply": reply}, nil
	default:
		return nil, fmt.Errorf("unsupported remote action")
	}
}

func sortConversations(conversations []task.Conversation) {
	sort.SliceStable(conversations, func(i, j int) bool {
		return conversations[i].UpdatedAt.After(conversations[j].UpdatedAt)
	})
}

func truncateUTF8Bytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return string(value[:end]) + "…"
}
