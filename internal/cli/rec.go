package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/sim4gh/nikte-cli/internal/platform"
	"github.com/spf13/cobra"
)

var (
	recDuration     int
	recSelect       bool
	recFormat       string
	recFPS          int
	recWidth        int
	recPermanent    bool
	recTTL          string
	recPublic       bool
	recPassword     string
)

const (
	maxRecDuration     = 60
	defaultRecDuration = 10
	defaultRecFPS      = 15
)

func addRecCommand() {
	recCmd := &cobra.Command{
		Use:   "rec",
		Short: "Record screen to GIF, MP4, or MOV",
		Long: `Record screen to GIF, MP4, or MOV

Examples:
  nk rec                      Record fullscreen 10s → GIF
    ├ -s                       Select region → record → GIF
    ├ -d 30                    Record for 30 seconds
    ├ -f mp4                   Record → MP4
    ├ -f mov                   Record → MOV (no ffmpeg needed)
    ├ -s -f mp4 -d 20          Select region, 20s, MP4
    └ -w 1280                  Scale to 1280px wide`,
		RunE: runRec,
	}

	recCmd.Flags().IntVarP(&recDuration, "duration", "d", defaultRecDuration, "Recording duration in seconds (max 60)")
	recCmd.Flags().BoolVarP(&recSelect, "select", "s", false, "Select screen region to record")
	recCmd.Flags().StringVarP(&recFormat, "format", "f", "gif", "Output format: gif, mp4, mov")
	recCmd.Flags().IntVar(&recFPS, "fps", defaultRecFPS, "Frame rate for GIF")
	recCmd.Flags().IntVarP(&recWidth, "width", "w", 0, "Scale output width in pixels (0 = original)")
	recCmd.Flags().BoolVar(&recPermanent, "permanent", false, "Keep forever (default: 24h TTL)")
	recCmd.Flags().StringVar(&recTTL, "ttl", defaultTTL, "Custom TTL (e.g., 1h, 7d)")
	recCmd.Flags().BoolVarP(&recPublic, "public", "p", false, "Create public share on add (Pro)")
	recCmd.Flags().StringVar(&recPassword, "password", "", "Password-protected share (Pro)")

	rootCmd.AddCommand(recCmd)
}

func runRec(cmd *cobra.Command, args []string) error {
	if !platform.IsRecordingSupported() {
		return fmt.Errorf("screen recording is only supported on macOS")
	}

	// Validate format
	switch recFormat {
	case "gif", "mp4", "mov":
		// ok
	default:
		return fmt.Errorf("unsupported format %q (use gif, mp4, or mov)", recFormat)
	}

	// ffmpeg required for gif and mp4
	if recFormat != "mov" && !platform.HasFFmpeg() {
		return fmt.Errorf("ffmpeg is required for %s format. Install with: brew install ffmpeg", recFormat)
	}

	// Cap duration
	if recDuration > maxRecDuration {
		fmt.Printf("Note: duration capped at %d seconds\n", maxRecDuration)
		recDuration = maxRecDuration
	}
	if recDuration <= 0 {
		recDuration = defaultRecDuration
	}

	// Record
	if recSelect {
		fmt.Println("Select area to record (Ctrl+C to stop)...")
	} else {
		fmt.Printf("Recording fullscreen for %d seconds\n", recDuration)
	}

	movPath, err := platform.RecordScreen(recDuration, recSelect)
	if err != nil {
		return err
	}
	if movPath == "" {
		fmt.Println("Recording cancelled")
		return nil
	}
	defer os.Remove(movPath)

	fmt.Println("Recording complete!")

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)

	// Convert if needed
	outputPath := movPath
	switch recFormat {
	case "gif":
		s.Suffix = " Converting to GIF..."
		s.Start()
		gifPath, err := platform.ConvertToGIF(movPath, recFPS, recWidth)
		s.Stop()
		if err != nil {
			return err
		}
		defer os.Remove(gifPath)
		outputPath = gifPath
		fmt.Println("GIF created!")

	case "mp4":
		s.Suffix = " Converting to MP4..."
		s.Start()
		mp4Path, err := platform.ConvertToMP4(movPath, recWidth)
		s.Stop()
		if err != nil {
			return err
		}
		defer os.Remove(mp4Path)
		outputPath = mp4Path
		fmt.Println("MP4 created!")

	case "mov":
		// No conversion needed
	}

	// Upload using existing file upload flow
	// Set the add flags so handleFileUpload picks them up
	addPermanent = recPermanent
	addTTL = recTTL
	addPublic = recPublic
	addPassword = recPassword

	s = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	return handleFileUpload(outputPath, s)
}
