package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/briandowns/spinner"
	"github.com/mdp/qrterminal/v3"
	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/sim4gh/nikte-cli/internal/platform"
	"github.com/sim4gh/nikte-cli/internal/upload"
	"github.com/sim4gh/nikte-cli/internal/util"
	"github.com/sim4gh/nikte-cli/internal/whatsapp"
	"github.com/spf13/cobra"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

var (
	waLsAll          bool
	waSendFullscreen bool
	waSendItem       string
)

// notLinkedError builds the "not linked" error, naming the profile when it is
// not the default so the user runs "nk wa link" against the right account.
func notLinkedError(profile int) error {
	if profile == 1 {
		return fmt.Errorf(`WhatsApp not linked. Run "nk wa link" first`)
	}
	return fmt.Errorf(`WhatsApp profile %d not linked. Run "nk wa link -p %d" first`, profile, profile)
}

func addWaCommands() {
	waCmd := &cobra.Command{
		Use:   "wa",
		Short: "WhatsApp messaging commands",
		Long: `WhatsApp messaging commands

Examples:
  nk wa link                          Link WhatsApp (scan QR code)
  nk wa send 7778887788 "Hello!"      Send a message
  nk wa send 7778887788               Send clipboard content
  nk wa ls                            Show unread messages
  nk wa ls --all                      Show all recent conversations
  nk wa status                        Check link status
  nk wa unlink                        Unlink WhatsApp
  nk wa link -p 2                     Link a second account (profile 2)
  nk wa send -p 2 7778887788 "Hi"     Send from profile 2
  nk wa status                        Show status of all profiles`,
	}

	linkCmd := &cobra.Command{
		Use:   "link",
		Short: "Link WhatsApp by scanning QR code",
		RunE:  runWaLink,
	}

	sendCmd := &cobra.Command{
		Use:   "send <number> [message|file|sc] [caption]",
		Short: "Send a WhatsApp message or image",
		Long: `Send a WhatsApp message, image, video or document to a phone number.

The second argument is auto-detected:
  - "sc"                      capture a screenshot and send it (macOS)
  - an existing file path     send that file (image/video/audio/document)
  - anything else             send it as a text message
  - omitted                   send clipboard content (image if present, else text)

Use --item <id> to forward an existing nikte item (text, file, or screenshot)
by its ID: text is sent as a message, files/screenshots are downloaded and sent
as media.

When sending a file or screenshot, any extra words become the caption.
Phone number should include country code (e.g., 14255687870 for US).

Examples:
  nk wa send 14255687870 "Hello!"               # text message
  nk wa send 14255687870                         # clipboard (image or text)
  nk wa send 14255687870 photo.png "nice!"       # image with caption
  nk wa send 14255687870 clip.mp4                # video
  nk wa send 14255687870 report.pdf              # document
  nk wa send 14255687870 sc "look at this"       # screenshot with caption
  nk wa send 14255687870 --item AB12             # forward a nikte item
  nk wa send 14255687870 --item AB12 "fyi"       # forward with a caption
  nk wa send +1-425-568-7870 "Hi"                # non-digits are stripped
  nk wa send -p 2 14255687870 "Hi"              # send from profile 2`,
		Args: cobra.MinimumNArgs(1),
		RunE: runWaSend,
	}
	sendCmd.Flags().BoolVarP(&waSendFullscreen, "fullscreen", "f", false, "Capture full screen instead of region select (for sc)")
	sendCmd.Flags().StringVar(&waSendItem, "item", "", "Forward a nikte item (text/file/screenshot) by its ID")

	lsCmd := &cobra.Command{
		Use:     "ls",
		Short:   "Show unread messages",
		Aliases: []string{"list"},
		Long: `Connect to WhatsApp and display unread messages from history sync.

By default, only shows conversations with unread messages (the red badge).
Use --all to show all recent conversations.

Examples:
  nk wa ls          # show unread messages
  nk wa ls --all    # show all recent conversations`,
		RunE: runWaLs,
	}
	lsCmd.Flags().BoolVar(&waLsAll, "all", false, "Show all recent conversations, not just unread")

	unlinkCmd := &cobra.Command{
		Use:   "unlink",
		Short: "Unlink WhatsApp and clear session",
		RunE:  runWaUnlink,
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Check WhatsApp link status",
		RunE:  runWaStatus,
	}

	waCmd.PersistentFlags().IntP("profile", "p", 1, "WhatsApp profile to use (1-4)")
	// Validate the profile once for every wa subcommand, before any DB/session
	// work. root.go has no PersistentPreRunE, so this is not shadowed.
	waCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		profile, _ := cmd.Flags().GetInt("profile")
		return whatsapp.ValidateProfile(profile)
	}

	waCmd.AddCommand(linkCmd, sendCmd, lsCmd, unlinkCmd, statusCmd)
	rootCmd.AddCommand(waCmd)
}

func runWaLink(cmd *cobra.Command, args []string) error {
	profile, _ := cmd.Flags().GetInt("profile")
	client, err := whatsapp.NewClient(profile, false)
	if err != nil {
		return fmt.Errorf("failed to initialize WhatsApp: %w", err)
	}
	defer client.Disconnect()

	if client.Store.ID != nil {
		fmt.Println("WhatsApp is already linked.")
		fmt.Println("Run \"nk wa unlink\" first to re-link.")
		return nil
	}

	// Listen for pairing events via event handler (primary success signal)
	pairSuccess := make(chan string, 1)
	pairError := make(chan error, 1)

	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.PairSuccess:
			select {
			case pairSuccess <- v.ID.String():
			default:
			}
		case *events.PairError:
			select {
			case pairError <- fmt.Errorf("pairing failed: %v", v.Error):
			default:
			}
		case *events.Connected:
			select {
			case pairSuccess <- "connected":
			default:
			}
		}
	})

	qrChan, err := client.GetQRChannel(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get QR channel: %w", err)
	}

	err = client.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	fmt.Println("\nScan this QR code with WhatsApp:")
	fmt.Println("  WhatsApp > Settings > Linked Devices > Link a Device")
	fmt.Println()

	for {
		select {
		case evt, ok := <-qrChan:
			if !ok {
				// QR channel closed — check if we paired via event handler
				select {
				case <-pairSuccess:
					fmt.Println("\nWhatsApp linked successfully!")
					return nil
				default:
					return fmt.Errorf("connection closed unexpectedly")
				}
			}
			switch evt.Event {
			case "code":
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			case "login":
				fmt.Println("\nWhatsApp linked successfully!")
				return nil
			case "timeout":
				return fmt.Errorf("QR code expired. Run \"nk wa link\" again")
			case "error":
				return fmt.Errorf("QR channel error. Run \"nk wa link\" again")
			}

		case <-pairSuccess:
			fmt.Println("\nWhatsApp linked successfully!")
			// Wait briefly for the session to be fully saved
			time.Sleep(1 * time.Second)
			return nil

		case pairErr := <-pairError:
			return pairErr
		}
	}
}

func runWaSend(cmd *cobra.Command, args []string) error {
	profile, _ := cmd.Flags().GetInt("profile")
	client, err := whatsapp.NewClient(profile, false)
	if err != nil {
		return err
	}

	if client.Store.ID == nil {
		return notLinkedError(profile)
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Connecting to WhatsApp..."
	s.Start()

	err = client.Connect()
	if err != nil {
		s.Stop()
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Disconnect()

	s.Stop()

	number := args[0]

	// --item forwards an existing nikte item; otherwise auto-detect from args.
	var msg *waE2E.Message
	var desc string
	if waSendItem != "" {
		caption := strings.Join(args[1:], " ")
		msg, desc, err = buildWaItemMessage(client, waSendItem, caption)
	} else {
		msg, desc, err = buildWaSendMessage(client, args[1:])
	}
	if err != nil {
		return err
	}

	jid := whatsapp.FormatNumber(number)

	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Sending " + desc + "..."
	s.Start()

	_, err = client.SendMessage(context.Background(), jid, msg)
	s.Stop()

	if err != nil {
		return fmt.Errorf("failed to send: %w", err)
	}

	fmt.Printf("%s sent to %s\n", capitalize(desc), number)
	return nil
}

// buildWaSendMessage resolves the WhatsApp message to send from the arguments
// after the phone number, auto-detecting screenshots, files, and text.
// It returns the message, a short human-readable description, and any error.
func buildWaSendMessage(client *whatsmeow.Client, rest []string) (*waE2E.Message, string, error) {
	// No argument: send clipboard content (image if present, otherwise text).
	if len(rest) == 0 {
		if platform.ClipboardHasImage() {
			data, err := platform.GetClipboardImage()
			if err == nil && len(data) > 0 {
				fmt.Println("Sending clipboard image")
				return buildWaMedia(client, data, "image/png", "", "clipboard.png")
			}
		}
		text, clipErr := clipboard.ReadAll()
		if clipErr != nil || strings.TrimSpace(text) == "" {
			return nil, "", fmt.Errorf("no message provided and clipboard is empty")
		}
		fmt.Printf("Sending clipboard content (%d chars)\n", len(text))
		return &waE2E.Message{Conversation: proto.String(text)}, "message", nil
	}

	first := rest[0]
	caption := strings.Join(rest[1:], " ")

	// "sc": capture a screenshot and send it.
	if first == "sc" {
		if !platform.IsScreenshotSupported() {
			return nil, "", fmt.Errorf("screenshot capture is only supported on macOS")
		}
		if !waSendFullscreen {
			fmt.Println("Select area for screenshot...")
		}
		data, err := platform.CaptureScreenshot(false, waSendFullscreen)
		if err != nil {
			return nil, "", err
		}
		if data == nil {
			return nil, "", fmt.Errorf("screenshot cancelled")
		}
		return buildWaMedia(client, data, "image/png", caption, "screenshot.png")
	}

	// Existing file: send as media (image/video/audio/document).
	// Expand a leading "~" so quoted paths like "~/Downloads/x.png" also work
	// (unquoted ones are already expanded by the shell).
	path := expandTilde(first)
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		if info.Size() == 0 {
			return nil, "", fmt.Errorf("cannot send empty file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", err
		}
		return buildWaMedia(client, data, upload.GetMimeType(path), caption, filepath.Base(path))
	}

	// Otherwise: plain text message (join all remaining args).
	return &waE2E.Message{Conversation: proto.String(strings.Join(rest, " "))}, "message", nil
}

// buildWaItemMessage forwards an existing nikte item (looked up by ID) over
// WhatsApp. Text shorts are sent as a message; file shorts, screenshots, and
// Pro files are downloaded and sent as media. The caption applies to media only.
func buildWaItemMessage(client *whatsmeow.Client, id, caption string) (*waE2E.Message, string, error) {
	// Try as a short first (text or file).
	if resp, err := api.Get("/shorts/" + id); err == nil && resp.StatusCode == 200 {
		var item struct {
			Type        string `json:"type"`
			Content     string `json:"content"`
			Filename    string `json:"filename"`
			ContentType string `json:"contentType"`
			DownloadURL string `json:"downloadUrl"`
		}
		if err := resp.Unmarshal(&item); err != nil {
			return nil, "", err
		}
		if item.Type == "file" {
			data, err := downloadBytes(item.DownloadURL)
			if err != nil {
				return nil, "", fmt.Errorf("failed to download item %q: %w", id, err)
			}
			fmt.Printf("Forwarding %s\n", item.Filename)
			return buildWaMedia(client, data, item.ContentType, caption, item.Filename)
		}
		// Text short: send the content as a message.
		text := item.Content
		if caption != "" {
			text = caption + "\n" + text
		}
		fmt.Printf("Forwarding text item %q\n", id)
		return &waE2E.Message{Conversation: proto.String(text)}, "message", nil
	}

	// Try as a screenshot.
	if resp, err := api.Get("/screenshots/" + id); err == nil && resp.StatusCode == 200 {
		var item struct {
			ContentType string `json:"contentType"`
			DownloadURL string `json:"downloadUrl"`
		}
		if err := resp.Unmarshal(&item); err != nil {
			return nil, "", err
		}
		data, err := downloadBytes(item.DownloadURL)
		if err != nil {
			return nil, "", fmt.Errorf("failed to download screenshot %q: %w", id, err)
		}
		contentType := item.ContentType
		if contentType == "" {
			contentType = "image/png"
		}
		fmt.Printf("Forwarding screenshot %q\n", id)
		return buildWaMedia(client, data, contentType, caption, "screenshot-"+id+".png")
	}

	// Try as a Pro file.
	if resp, err := api.Get("/files/" + id); err == nil && resp.StatusCode == 200 {
		var item struct {
			Filename    string `json:"filename"`
			ContentType string `json:"contentType"`
			DownloadURL string `json:"downloadUrl"`
		}
		if err := resp.Unmarshal(&item); err != nil {
			return nil, "", err
		}
		data, err := downloadBytes(item.DownloadURL)
		if err != nil {
			return nil, "", fmt.Errorf("failed to download file %q: %w", id, err)
		}
		fmt.Printf("Forwarding %s\n", item.Filename)
		return buildWaMedia(client, data, item.ContentType, caption, item.Filename)
	}

	return nil, "", fmt.Errorf("no nikte item found with ID %q (it may have expired)", id)
}

// buildWaMedia uploads media bytes to WhatsApp and builds the matching message
// type based on the MIME type (image, video, audio, or document fallback).
func buildWaMedia(client *whatsmeow.Client, data []byte, mimeType, caption, filename string) (*waE2E.Message, string, error) {
	kind := mimeType
	if i := strings.Index(kind, "/"); i != -1 {
		kind = kind[:i]
	}

	var mediaType whatsmeow.MediaType
	switch kind {
	case "image":
		mediaType = whatsmeow.MediaImage
	case "video":
		mediaType = whatsmeow.MediaVideo
	case "audio":
		mediaType = whatsmeow.MediaAudio
	default:
		mediaType = whatsmeow.MediaDocument
	}

	fmt.Printf("Uploading %s (%s)\n", filename, util.FormatBytes(int64(len(data))))
	resp, err := client.Upload(context.Background(), data, mediaType)
	if err != nil {
		return nil, "", fmt.Errorf("failed to upload media: %w", err)
	}

	switch mediaType {
	case whatsmeow.MediaImage:
		m := &waE2E.ImageMessage{
			Mimetype:      proto.String(mimeType),
			URL:           &resp.URL,
			DirectPath:    &resp.DirectPath,
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    &resp.FileLength,
		}
		if caption != "" {
			m.Caption = proto.String(caption)
		}
		return &waE2E.Message{ImageMessage: m}, "image", nil

	case whatsmeow.MediaVideo:
		m := &waE2E.VideoMessage{
			Mimetype:      proto.String(mimeType),
			URL:           &resp.URL,
			DirectPath:    &resp.DirectPath,
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    &resp.FileLength,
		}
		if caption != "" {
			m.Caption = proto.String(caption)
		}
		return &waE2E.Message{VideoMessage: m}, "video", nil

	case whatsmeow.MediaAudio:
		m := &waE2E.AudioMessage{
			Mimetype:      proto.String(mimeType),
			URL:           &resp.URL,
			DirectPath:    &resp.DirectPath,
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    &resp.FileLength,
		}
		return &waE2E.Message{AudioMessage: m}, "audio", nil

	default:
		m := &waE2E.DocumentMessage{
			Mimetype:      proto.String(mimeType),
			FileName:      proto.String(filename),
			URL:           &resp.URL,
			DirectPath:    &resp.DirectPath,
			MediaKey:      resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    &resp.FileLength,
		}
		if caption != "" {
			m.Caption = proto.String(caption)
		}
		return &waE2E.Message{DocumentMessage: m}, "document", nil
	}
}

// chatMessages holds messages collected for a single chat.
type chatMessages struct {
	Name     string
	JID      string
	IsGroup  bool
	Messages []chatMsg
	// HistorySync metadata (supplementary)
	HistoryUnreadCount uint32
	HistoryName        string
}

// chatMsg holds a single message from events.Message.
type chatMsg struct {
	Time   time.Time
	Sender string
	Text   string
	FromMe bool
}

func runWaLs(cmd *cobra.Command, args []string) error {
	profile, _ := cmd.Flags().GetInt("profile")
	client, err := whatsapp.NewClient(profile, false)
	if err != nil {
		return err
	}

	if client.Store.ID == nil {
		return notLinkedError(profile)
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Connecting and syncing messages..."
	s.Start()

	var mu sync.Mutex
	chats := make(map[types.JID]*chatMessages)        // offline messages by chat
	historyMeta := make(map[string]*histSyncConvMeta) // history sync metadata by JID string
	done := make(chan struct{}, 1)

	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.OfflineSyncPreview:
			// Informational — server will send v.Messages offline messages
		case *events.Message:
			text := extractMessageText(v.Message)
			if text == "" {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			chat, ok := chats[v.Info.Chat]
			if !ok {
				name := v.Info.PushName
				if v.Info.IsGroup {
					name = v.Info.Chat.String() // placeholder, may be enriched by history sync
				}
				chat = &chatMessages{
					Name:    name,
					JID:     v.Info.Chat.String(),
					IsGroup: v.Info.IsGroup,
				}
				chats[v.Info.Chat] = chat
			}
			// For 1:1 chats, use PushName as chat name if we don't have one yet
			if !v.Info.IsGroup && chat.Name == "" && v.Info.PushName != "" {
				chat.Name = v.Info.PushName
			}
			chat.Messages = append(chat.Messages, chatMsg{
				Time:   v.Info.Timestamp,
				Sender: v.Info.PushName,
				Text:   text,
				FromMe: v.Info.IsFromMe,
			})
		case *events.OfflineSyncCompleted:
			select {
			case done <- struct{}{}:
			default:
			}
		case *events.HistorySync:
			if v.Data == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, conv := range v.Data.GetConversations() {
				jid := conv.GetID()
				name := conv.GetDisplayName()
				if name == "" {
					name = conv.GetName()
				}
				historyMeta[jid] = &histSyncConvMeta{
					Name:        name,
					UnreadCount: conv.GetUnreadCount(),
				}
			}
		}
	})

	err = client.Connect()
	if err != nil {
		s.Stop()
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Disconnect()

	// Wait for OfflineSyncCompleted or timeout
	select {
	case <-done:
		// Give a brief extra window for any trailing history sync
		time.Sleep(2 * time.Second)
	case <-time.After(15 * time.Second):
	}
	s.Stop()

	mu.Lock()
	defer mu.Unlock()

	// Merge history sync metadata into chats
	for jid, chat := range chats {
		if meta, ok := historyMeta[jid.String()]; ok {
			if meta.Name != "" && (chat.IsGroup || chat.Name == "" || chat.Name == jid.String()) {
				chat.Name = meta.Name
			}
			chat.HistoryUnreadCount = meta.UnreadCount
			chat.HistoryName = meta.Name
		}
		// Fallback: if still no name, use JID
		if chat.Name == "" {
			chat.Name = jid.String()
		}
	}

	// Build sorted list of chats to display
	var display []*chatMessages
	for _, chat := range chats {
		// Filter out chats with only FromMe messages (unless --all)
		hasIncoming := false
		for _, msg := range chat.Messages {
			if !msg.FromMe {
				hasIncoming = true
				break
			}
		}
		if !waLsAll && !hasIncoming {
			continue
		}
		display = append(display, chat)
	}

	if len(display) == 0 {
		if waLsAll {
			fmt.Println("No conversations found.")
		} else {
			fmt.Println("No unread messages.")
		}
		return nil
	}

	// Sort by most recent message (newest chat first)
	sort.Slice(display, func(i, j int) bool {
		ti := display[i].Messages[len(display[i].Messages)-1].Time
		tj := display[j].Messages[len(display[j].Messages)-1].Time
		return ti.After(tj)
	})

	for _, chat := range display {
		printChat(chat)
	}

	return nil
}

// histSyncConvMeta holds supplementary metadata from history sync.
type histSyncConvMeta struct {
	Name        string
	UnreadCount uint32
}

// printChat renders a single chat's messages.
func printChat(chat *chatMessages) {
	// Sort messages by timestamp (oldest first)
	sort.Slice(chat.Messages, func(i, j int) bool {
		return chat.Messages[i].Time.Before(chat.Messages[j].Time)
	})

	// Count incoming messages
	incomingCount := 0
	for _, msg := range chat.Messages {
		if !msg.FromMe {
			incomingCount++
		}
	}

	// Use history sync unread count if available, otherwise count of incoming messages
	unreadCount := incomingCount
	if chat.HistoryUnreadCount > 0 {
		unreadCount = int(chat.HistoryUnreadCount)
	}

	// Header line
	if unreadCount > 0 {
		fmt.Printf("\n%s — %d unread\n", chat.Name, unreadCount)
	} else {
		fmt.Printf("\n%s\n", chat.Name)
	}

	for _, msg := range chat.Messages {
		if msg.FromMe {
			continue
		}
		ts := msg.Time.Local().Format("15:04")
		if chat.IsGroup && msg.Sender != "" {
			fmt.Printf("  [%s] %s: %s\n", ts, msg.Sender, truncateMsg(msg.Text, 70))
		} else {
			fmt.Printf("  [%s] %s\n", ts, truncateMsg(msg.Text, 70))
		}
	}
}

func runWaUnlink(cmd *cobra.Command, args []string) error {
	profile, _ := cmd.Flags().GetInt("profile")
	if !whatsapp.IsLinked(profile) {
		fmt.Println("WhatsApp is not linked.")
		return nil
	}

	client, err := whatsapp.NewClient(profile, false)
	if err != nil {
		// If we can't create client, just delete the DB
		if delErr := whatsapp.DeleteDB(profile); delErr != nil {
			return fmt.Errorf("failed to delete session: %w", delErr)
		}
		fmt.Println("WhatsApp session cleared.")
		return nil
	}

	// Try to properly logout from WhatsApp servers
	if client.Store.ID != nil {
		if connErr := client.Connect(); connErr == nil {
			_ = client.Logout(context.Background())
		}
		client.Disconnect()
	}

	if err := whatsapp.DeleteDB(profile); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	fmt.Println("WhatsApp unlinked successfully.")
	return nil
}

// formatProfileLine renders one row of the multi-profile status overview.
func formatProfileLine(profile int, linked bool, id string) string {
	if !linked {
		return fmt.Sprintf("  %d  Not linked", profile)
	}
	return fmt.Sprintf("  %d  Linked      (%s)", profile, id)
}

func runWaStatus(cmd *cobra.Command, args []string) error {
	profile, _ := cmd.Flags().GetInt("profile")

	// No explicit -p: fast, local-only overview of all profiles (no network).
	if !cmd.Flags().Changed("profile") {
		fmt.Println("WhatsApp profiles:")
		for p := 1; p <= 4; p++ {
			linked, id, _ := whatsapp.ProfileStatus(p)
			fmt.Println(formatProfileLine(p, linked, id))
		}
		return nil
	}

	// Explicit -p N: detailed status for that profile (verifies live connection).
	if !whatsapp.IsLinked(profile) {
		fmt.Printf("WhatsApp profile %d: Not linked\n", profile)
		fmt.Printf("Run \"nk wa link -p %d\" to connect this account.\n", profile)
		return nil
	}

	client, err := whatsapp.NewClient(profile, false)
	if err != nil {
		return err
	}

	if client.Store.ID == nil {
		fmt.Printf("WhatsApp profile %d: Not linked\n", profile)
		fmt.Printf("Run \"nk wa link -p %d\" to connect this account.\n", profile)
		return nil
	}

	fmt.Printf("WhatsApp profile %d: Linked\n", profile)
	fmt.Printf("  Device: %s\n", client.Store.ID.String())

	// Try a quick connect to verify session is still valid
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Verifying connection..."
	s.Start()

	err = client.Connect()
	s.Stop()

	if err != nil {
		fmt.Println("  Status: Session expired (re-link with \"nk wa link\")")
	} else {
		fmt.Println("  Status: Connected")
		client.Disconnect()
	}

	return nil
}

// extractMessageText extracts user-visible text from a WhatsApp message.
// Returns empty string for protocol/system messages that should be skipped.
func extractMessageText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}

	// Unwrap container message types
	if msg.GetEphemeralMessage() != nil && msg.GetEphemeralMessage().GetMessage() != nil {
		msg = msg.GetEphemeralMessage().GetMessage()
	}
	if msg.GetDeviceSentMessage() != nil && msg.GetDeviceSentMessage().GetMessage() != nil {
		msg = msg.GetDeviceSentMessage().GetMessage()
	}
	if msg.GetViewOnceMessage() != nil && msg.GetViewOnceMessage().GetMessage() != nil {
		msg = msg.GetViewOnceMessage().GetMessage()
	}
	if msg.GetViewOnceMessageV2() != nil && msg.GetViewOnceMessageV2().GetMessage() != nil {
		msg = msg.GetViewOnceMessageV2().GetMessage()
	}

	// Extract text from known message types
	if t := msg.GetConversation(); t != "" {
		return t
	}
	if t := msg.GetExtendedTextMessage(); t != nil && t.GetText() != "" {
		return t.GetText()
	}
	if t := msg.GetImageMessage(); t != nil {
		if cap := t.GetCaption(); cap != "" {
			return "[Image] " + cap
		}
		return "[Image]"
	}
	if t := msg.GetVideoMessage(); t != nil {
		if cap := t.GetCaption(); cap != "" {
			return "[Video] " + cap
		}
		return "[Video]"
	}
	if msg.GetDocumentMessage() != nil {
		name := msg.GetDocumentMessage().GetFileName()
		if name != "" {
			return "[Document] " + name
		}
		return "[Document]"
	}
	if msg.GetAudioMessage() != nil {
		if msg.GetAudioMessage().GetPTT() {
			return "[Voice note]"
		}
		return "[Audio]"
	}
	if msg.GetStickerMessage() != nil {
		return "[Sticker]"
	}
	if msg.GetContactMessage() != nil {
		return "[Contact] " + msg.GetContactMessage().GetDisplayName()
	}
	if msg.GetLocationMessage() != nil {
		return "[Location]"
	}
	if msg.GetLiveLocationMessage() != nil {
		return "[Live location]"
	}
	if msg.GetReactionMessage() != nil {
		return msg.GetReactionMessage().GetText()
	}
	if msg.GetPollCreationMessage() != nil {
		return "[Poll] " + msg.GetPollCreationMessage().GetName()
	}

	// Skip protocol/system messages (no user content)
	if msg.GetProtocolMessage() != nil ||
		msg.GetSenderKeyDistributionMessage() != nil ||
		msg.GetPollUpdateMessage() != nil {
		return ""
	}

	// Unknown message type — skip rather than show "[Message]"
	return ""
}

// expandTilde replaces a leading "~" or "~/" with the user's home directory.
// Other paths are returned unchanged. This makes quoted paths behave like the
// shell's own tilde expansion for unquoted arguments.
func expandTilde(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path[1:], "/"))
		}
	}
	return path
}

func truncateMsg(s string, maxLen int) string {
	// Replace newlines with spaces for single-line display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
