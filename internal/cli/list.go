package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/olekukonko/tablewriter"
	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/sim4gh/nikte-cli/internal/util"
	"github.com/spf13/cobra"
)

var (
	listType        string
	listSearch      string
	listLimit       string
	listSort        string
	listRaw         bool
	listInteractive bool
)

// Item represents a unified item
type Item struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Preview   string `json:"preview,omitempty"`
	Filename  string `json:"filename,omitempty"`
	Size      int64  `json:"size"`
	ExpiresAt int64  `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
	Source    string `json:"source"`
}

func addListCommand() {
	listCmd := &cobra.Command{
		Use:   "ls",
		Short: "List all items",
		Long: `List all items

Examples:
  nk ls                       List all items
    ├ -i, --interactive        Navigable list (arrows, copy, delete)
    ├ --type text              Show only text items
    ├ --search "important"     Search for "important"
    ├ --limit 5 --sort size    Top 5 by size
    └ --raw | jq ".[]"         JSON output for scripting`,
		Aliases: []string{"list"},
		RunE:    runList,
	}

	listCmd.Flags().StringVarP(&listType, "type", "t", "", "Filter by type: text, file, screenshot, pro")
	listCmd.Flags().StringVarP(&listSearch, "search", "s", "", "Search in content/filename")
	listCmd.Flags().StringVarP(&listLimit, "limit", "l", "", "Limit number of results")
	listCmd.Flags().StringVar(&listSort, "sort", "date", "Sort by: size, date, expiry")
	listCmd.Flags().BoolVar(&listRaw, "raw", false, "Output as JSON (for piping)")
	listCmd.Flags().BoolVarP(&listInteractive, "interactive", "i", false, "Interactive navigable list")

	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Fetching items..."
	s.Start()

	allItems := fetchAllItems()

	s.Stop()

	// Apply filters
	if listType != "" {
		allItems = filterByType(allItems, listType)
	}

	if listSearch != "" {
		allItems = filterBySearch(allItems, listSearch)
	}

	// Sort items
	allItems = sortItems(allItems, listSort)

	// Interactive mode: launch the navigable TUI with the filtered/sorted set.
	if listInteractive {
		return runListTUI(allItems)
	}

	// Apply limit
	totalBeforeLimit := len(allItems)
	if listLimit != "" {
		limit, err := strconv.Atoi(listLimit)
		if err != nil || limit <= 0 {
			return fmt.Errorf("invalid limit: must be a positive number")
		}
		if limit < len(allItems) {
			allItems = allItems[:limit]
		}
	}

	// Output as JSON if --raw flag is set
	if listRaw {
		data, err := json.MarshalIndent(allItems, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Display the table
	fmt.Println()
	displayItemsTable(allItems)

	// Show summary
	textCount := countByType(allItems, "text")
	fileCount := countByType(allItems, "file")
	scCount := countByType(allItems, "screenshot")
	proCount := countByType(allItems, "profile")

	var parts []string
	if textCount > 0 {
		parts = append(parts, fmt.Sprintf("%d text", textCount))
	}
	if fileCount > 0 {
		parts = append(parts, fmt.Sprintf("%d file", fileCount))
	}
	if scCount > 0 {
		parts = append(parts, fmt.Sprintf("%d screenshot", scCount))
	}
	if proCount > 0 {
		parts = append(parts, fmt.Sprintf("%d pro file", proCount))
	}

	limitInfo := ""
	if listLimit != "" && totalBeforeLimit > len(allItems) {
		limitInfo = fmt.Sprintf(" (showing %d of %d)", len(allItems), totalBeforeLimit)
	}

	summary := "none"
	if len(parts) > 0 {
		summary = strings.Join(parts, ", ")
	}
	fmt.Printf("\nTotal: %d items (%s)%s\n", len(allItems), summary, limitInfo)

	// Show active filters
	var activeFilters []string
	if listType != "" {
		activeFilters = append(activeFilters, fmt.Sprintf("type=%s", listType))
	}
	if listSearch != "" {
		activeFilters = append(activeFilters, fmt.Sprintf("search=\"%s\"", listSearch))
	}
	if listSort != "" && listSort != "date" {
		activeFilters = append(activeFilters, fmt.Sprintf("sort=%s", listSort))
	}

	if len(activeFilters) > 0 {
		fmt.Printf("Filters: %s\n", strings.Join(activeFilters, ", "))
	}

	fmt.Println("\nTypes: Text, File, Screenshot, Pro")

	return nil
}

// fetchAllItems fetches shorts, screenshots, and files concurrently and returns
// them combined (unfiltered, unsorted).
func fetchAllItems() []Item {
	shortsChan := make(chan []Item)
	screenshotsChan := make(chan []Item)
	filesChan := make(chan []Item)

	go func() { shortsChan <- fetchShorts() }()
	go func() { screenshotsChan <- fetchScreenshots() }()
	go func() { filesChan <- fetchFiles() }()

	shorts := <-shortsChan
	screenshots := <-screenshotsChan
	files := <-filesChan

	return append(append(shorts, screenshots...), files...)
}

func fetchShorts() []Item {
	resp, err := api.Get("/shorts")
	if err != nil || resp.StatusCode != 200 {
		return nil
	}

	var result struct {
		Shorts []struct {
			ShortID        string `json:"shortId"`
			ID             string `json:"id"`
			Type           string `json:"type"`
			ContentPreview string `json:"contentPreview"`
			Content        string `json:"content"`
			Filename       string `json:"filename"`
			FileSize       int64  `json:"fileSize"`
			ExpiresAt      int64  `json:"expiresAt"`
			CreatedAt      string `json:"createdAt"`
		} `json:"shorts"`
	}
	if err := resp.Unmarshal(&result); err != nil {
		return nil
	}

	var items []Item
	for _, s := range result.Shorts {
		id := s.ShortID
		if id == "" {
			id = s.ID
		}
		itemType := "text"
		if s.Type == "file" {
			itemType = "file"
		}
		preview := s.ContentPreview
		if preview == "" {
			preview = s.Content
		}
		size := s.FileSize
		if size == 0 && s.Content != "" {
			size = int64(len(s.Content))
		}

		items = append(items, Item{
			ID:        id,
			Type:      itemType,
			Preview:   preview,
			Filename:  s.Filename,
			Size:      size,
			ExpiresAt: s.ExpiresAt,
			CreatedAt: s.CreatedAt,
			Source:    "short",
		})
	}
	return items
}

func fetchScreenshots() []Item {
	resp, err := api.Get("/screenshots")
	if err != nil || resp.StatusCode != 200 {
		return nil
	}

	var result struct {
		Screenshots []struct {
			ScreenshotID string `json:"screenshotId"`
			ID           string `json:"id"`
			Filename     string `json:"filename"`
			Size         int64  `json:"size"`
			ExpiresAt    int64  `json:"expiresAt"`
			CreatedAt    string `json:"createdAt"`
		} `json:"screenshots"`
	}
	if err := resp.Unmarshal(&result); err != nil {
		return nil
	}

	var items []Item
	for _, sc := range result.Screenshots {
		id := sc.ScreenshotID
		if id == "" {
			id = sc.ID
		}
		filename := sc.Filename
		if filename == "" {
			filename = "screenshot-" + id
		}

		items = append(items, Item{
			ID:        id,
			Type:      "screenshot",
			Filename:  filename,
			Size:      sc.Size,
			ExpiresAt: sc.ExpiresAt,
			CreatedAt: sc.CreatedAt,
			Source:    "screenshot",
		})
	}
	return items
}

func fetchFiles() []Item {
	resp, err := api.Get("/files")
	if err != nil || resp.StatusCode != 200 {
		return nil
	}

	var result struct {
		Files []struct {
			FileID    string `json:"fileId"`
			ID        string `json:"id"`
			Filename  string `json:"filename"`
			Size      int64  `json:"size"`
			ExpiresAt int64  `json:"expiresAt"`
			CreatedAt string `json:"createdAt"`
		} `json:"files"`
	}
	if err := resp.Unmarshal(&result); err != nil {
		return nil
	}

	var items []Item
	for _, f := range result.Files {
		id := f.FileID
		if id == "" {
			id = f.ID
		}

		items = append(items, Item{
			ID:        id,
			Type:      "profile",
			Filename:  f.Filename,
			Size:      f.Size,
			ExpiresAt: f.ExpiresAt,
			CreatedAt: f.CreatedAt,
			Source:    "file",
		})
	}
	return items
}

func filterByType(items []Item, typeFilter string) []Item {
	typeFilter = strings.ToLower(typeFilter)

	typeMap := map[string]string{
		"text":       "text",
		"file":       "file",
		"screenshot": "screenshot",
		"pro":        "profile",
	}

	mappedType, ok := typeMap[typeFilter]
	if !ok {
		fmt.Fprintf(os.Stderr, "Invalid type: %s. Valid types: text, file, screenshot, pro\n", typeFilter)
		return items
	}

	var filtered []Item
	for _, item := range items {
		if item.Type == mappedType {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterBySearch(items []Item, query string) []Item {
	query = strings.ToLower(query)

	var filtered []Item
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Preview), query) ||
			strings.Contains(strings.ToLower(item.Filename), query) ||
			strings.Contains(strings.ToLower(item.ID), query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func sortItems(items []Item, sortField string) []Item {
	sortField = strings.ToLower(sortField)

	switch sortField {
	case "size":
		sort.Slice(items, func(i, j int) bool {
			return items[i].Size > items[j].Size
		})
	case "expiry":
		sort.Slice(items, func(i, j int) bool {
			if items[i].ExpiresAt == 0 {
				return false // Permanent items at end
			}
			if items[j].ExpiresAt == 0 {
				return true
			}
			return items[i].ExpiresAt < items[j].ExpiresAt
		})
	default: // date
		sort.Slice(items, func(i, j int) bool {
			return items[i].CreatedAt > items[j].CreatedAt
		})
	}

	return items
}

func countByType(items []Item, itemType string) int {
	count := 0
	for _, item := range items {
		if item.Type == itemType {
			count++
		}
	}
	return count
}

func displayItemsTable(items []Item) {
	if len(items) == 0 {
		fmt.Println("No items found.")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Type", "Content / Filename", "Size", "Date", "Expires"})
	table.SetBorder(true)
	table.SetAutoWrapText(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)

	for _, item := range items {
		var typeName, contentDisplay, sizeDisplay, dateDisplay, expiry string

		switch item.Type {
		case "text":
			typeName = "Text"
			contentDisplay = util.Truncate(util.ReplaceNewlines(item.Preview), 38)
		case "file":
			typeName = "File"
			contentDisplay = util.Truncate(item.Filename, 38)
		case "screenshot":
			typeName = "Screenshot"
			contentDisplay = util.Truncate(item.Filename, 38)
		case "profile":
			typeName = "Pro"
			contentDisplay = util.Truncate(item.Filename, 38)
		default:
			typeName = "?"
			if item.Preview != "" {
				contentDisplay = util.Truncate(item.Preview, 38)
			} else {
				contentDisplay = util.Truncate(item.Filename, 38)
			}
		}

		if item.Size > 0 {
			sizeDisplay = util.FormatBytes(item.Size)
		}

		if item.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, item.CreatedAt); err == nil {
				dateDisplay = t.Local().Format("Jan 02")
			} else if t, err := time.Parse("2006-01-02T15:04:05.000Z", item.CreatedAt); err == nil {
				dateDisplay = t.Local().Format("Jan 02")
			}
		}

		if item.ExpiresAt > 0 {
			expiry = util.FormatExpiry(item.ExpiresAt)
		} else {
			expiry = "-"
		}

		table.Append([]string{item.ID, typeName, contentDisplay, sizeDisplay, dateDisplay, expiry})
	}

	table.Render()
}
