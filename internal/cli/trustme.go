package cli

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/sim4gh/nikte-cli/internal/api"
	"github.com/sim4gh/nikte-cli/internal/upload"
	"github.com/sim4gh/nikte-cli/internal/util"
	"github.com/spf13/cobra"
)

const maxDirectUploadBytes = 5 * 1024 * 1024 // 5MB

func addTrustMeCommand() {
	trustMeCmd := &cobra.Command{
		Use:   "trustme <token> <file>",
		Short: "Upload a file using a trust token (no auth required)",
		Long: `Upload a file to someone's account using their trust token

Examples:
  nk trustme <token> <file>   Upload file using trust token
    ├ abc123 photo.jpg         Upload image
    └ abc123 large-video.mp4   Upload large file (multipart)`,
		Args: cobra.ExactArgs(2),
		RunE: runTrustMe,
	}

	rootCmd.AddCommand(trustMeCmd)
}

func runTrustMe(cmd *cobra.Command, args []string) error {
	token := args[0]
	filePath := args[1]

	// Validate file
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("file not found: %s", filePath)
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("cannot upload a directory")
	}
	if fileInfo.Size() == 0 {
		return fmt.Errorf("cannot upload empty file")
	}

	filename := filepath.Base(filePath)
	contentType := upload.GetMimeType(filePath)

	fmt.Printf("File: %s\n", filename)
	fmt.Printf("Size: %s\n", util.FormatBytes(fileInfo.Size()))
	fmt.Printf("Type: %s\n", contentType)

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)

	// Read file
	s.Suffix = " Reading file..."
	s.Start()
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		s.Stop()
		return err
	}
	s.Stop()

	if fileInfo.Size() <= maxDirectUploadBytes {
		// Direct upload for small files
		return trustDirectUpload(token, filename, contentType, fileData, s)
	}

	// Multipart upload for large files
	return trustMultipartUpload(token, filename, contentType, fileData, fileInfo.Size(), s)
}

func trustDirectUpload(token, filename, contentType string, fileData []byte, s *spinner.Spinner) error {
	s.Suffix = " Uploading file..."
	s.Start()

	base64Data := base64.StdEncoding.EncodeToString(fileData)

	resp, err := api.PutNoAuth("/trust/"+token, map[string]interface{}{
		"filename":    filename,
		"contentType": contentType,
		"content":     base64Data,
	})
	if err != nil {
		s.Stop()
		return err
	}

	s.Stop()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("failed to upload: %s", resp.GetString("message"))
	}

	var result struct {
		ShortID   string `json:"shortId"`
		ExpiresAt int64  `json:"expiresAt"`
	}
	if err := resp.Unmarshal(&result); err != nil {
		return err
	}

	fmt.Println("Upload complete!")
	fmt.Println()
	if result.ShortID != "" {
		fmt.Printf("ID: %s\n", result.ShortID)
	}
	if result.ExpiresAt > 0 {
		fmt.Printf("Expires: %s\n", util.FormatExpiryTime(result.ExpiresAt))
	}

	return nil
}

func trustMultipartUpload(token, filename, contentType string, fileData []byte, fileSize int64, s *spinner.Spinner) error {
	// Initialize multipart upload
	s.Suffix = " Initializing upload..."
	s.Start()

	initResp, err := api.PostNoAuth("/trust/"+token+"/upload", map[string]interface{}{
		"filename":    filename,
		"contentType": contentType,
		"fileSize":    fileSize,
	})
	if err != nil {
		s.Stop()
		return err
	}

	if initResp.StatusCode != 200 && initResp.StatusCode != 201 {
		s.Stop()
		return fmt.Errorf("failed to initialize upload: %s", initResp.GetString("message"))
	}

	var initResult struct {
		ShortID       string `json:"shortId"`
		PresignedUrls []struct {
			PartNumber int    `json:"partNumber"`
			URL        string `json:"url"`
		} `json:"presignedUrls"`
		PartSize  int   `json:"partSize"`
		ExpiresAt int64 `json:"expiresAt"`
	}
	if err := initResp.Unmarshal(&initResult); err != nil {
		s.Stop()
		return err
	}

	s.Stop()
	fmt.Printf("Upload initialized (ID: %s)\n", initResult.ShortID)

	// Convert presigned URLs to upload.PresignedURL type
	presignedUrls := make([]upload.PresignedURL, len(initResult.PresignedUrls))
	for i, pu := range initResult.PresignedUrls {
		presignedUrls[i] = upload.PresignedURL{
			PartNumber: pu.PartNumber,
			URL:        pu.URL,
		}
	}

	// Upload parts
	totalParts := len(presignedUrls)
	s.Suffix = fmt.Sprintf(" Uploading 0/%d parts...", totalParts)
	s.Start()

	completedParts, err := upload.UploadParts(presignedUrls, fileData, initResult.PartSize, func(completed, total int, completedBytes, totalBytes int64) {
		progress := util.CreateProgressBar(completedBytes, totalBytes, 30)
		s.Suffix = fmt.Sprintf(" Uploading %d/%d parts... %s %s/%s", completed, total, progress, util.FormatBytes(completedBytes), util.FormatBytes(totalBytes))
	})
	if err != nil {
		s.Stop()
		return err
	}

	s.Stop()
	fmt.Printf("Uploaded %d parts\n", totalParts)

	// Complete multipart upload
	s.Suffix = " Finalizing upload..."
	s.Start()

	completeResp, err := api.PostNoAuth("/trust/"+token+"/complete", map[string]interface{}{
		"shortId": initResult.ShortID,
		"parts":   completedParts,
	})
	if err != nil {
		s.Stop()
		return err
	}

	if completeResp.StatusCode != 200 {
		s.Stop()
		return fmt.Errorf("failed to complete upload: %s", completeResp.GetString("message"))
	}

	s.Stop()
	fmt.Println("Upload complete!")
	fmt.Println()
	fmt.Printf("ID: %s\n", initResult.ShortID)
	if initResult.ExpiresAt > 0 {
		fmt.Printf("Expires: %s\n", util.FormatExpiryTime(initResult.ExpiresAt))
	}

	return nil
}
