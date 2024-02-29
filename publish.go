package plasmactlpublish

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/launchrctl/launchr/pkg/cli"

	"github.com/go-git/go-git/v5"
)

func getRepoInfo() (repoName, lastCommitShortSHA string, err error) {
	// Open repository
	r, err := git.PlainOpen(".")
	if err != nil {
		return "", "", err
	}

	// Get repository name
	remote, err := r.Remote("origin")
	if err != nil {
		return "", "", err
	}
	repoName = remote.Config().URLs[0]
	repoName = filepath.Base(repoName)
	repoName = repoName[:len(repoName)-4]

	// Get last commit information
	ref, err := r.Head()
	if err != nil {
		return "", "", err
	}
	lastCommitShortSHA = ref.Hash().String()[:7]

	return repoName, lastCommitShortSHA, nil
}

func listFiles(dir string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	cli.Println("Listing files in %s:", dir)
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		info, err := file.Info()
		if err != nil {
			return err
		}
		size := humanReadableSize(info.Size())
		cli.Println("%s %10s %s %s", info.Mode(), size, info.ModTime().Format(time.Stamp), file.Name())
	}

	return nil
}

func humanReadableSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func isURLAccessible(url string, code *int) bool {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}

	defer resp.Body.Close()
	*code = resp.StatusCode
	return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
}
