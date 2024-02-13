// Package plasmactlpublish implements a publish launchr plugin
package plasmactlpublish

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"syscall"

	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin is launchr plugin providing bump action.
type Plugin struct{}

// PluginInfo implements launchr.Plugin interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{}
}

// OnAppInit implements launchr.Plugin interface.
func (p *Plugin) OnAppInit(_ launchr.App) error {
	return nil
}

// CobraAddCommands implements launchr.CobraPlugin interface to provide bump functionality.
func (p *Plugin) CobraAddCommands(rootCmd *cobra.Command) error {
	var username string
	var password string

	var pblCmd = &cobra.Command{
		Use:   "publish",
		Short: "Upload local artifact archive to private repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Don't show usage help on a runtime error.
			cmd.SilenceUsage = true

			return publish(username, password)
		},
	}

	pblCmd.Flags().StringVarP(&username, "username", "", "", "Username")
	pblCmd.Flags().StringVarP(&password, "password", "", "", "Password")
	rootCmd.AddCommand(pblCmd)
	return nil
}

func publish(username, password string) error {
	// If username or password is empty, prompt the user to enter them
	if username == "" {
		fmt.Print("Artifacts repository username: ")
		_, err := fmt.Scanln(&username)
		if err != nil {
			return err
		}
	}

	if password == "" {
		fmt.Print("Artifacts repository password: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return err
		}
		password = string(passwordBytes)
		fmt.Println()
	}

	// Get repository information
	repoName, lastCommitShortSHA, err := getRepoInfo()
	if err != nil {
		log.Debug("%s", err)
		return errors.New("error getting repository information")
	}

	// Construct artifact file name
	archiveFile := fmt.Sprintf("%s-%s-plasma-src.tar.gz", repoName, lastCommitShortSHA)

	// Variables
	artifactDir := ".compose/artifacts"
	artifactPath := filepath.Join(artifactDir, archiveFile)
	artifactsRepositoryDomain := "https://repositories.skilld.cloud"

	// Check if the other repository is accessible
	var accessibilityCode int
	if isURLAccessible("http://repositories.interaction.svc.skilld:8081", &accessibilityCode) {
		artifactsRepositoryDomain = "http://repositories.interaction.svc.skilld:8081"
	}

	artifactArchiveURL := fmt.Sprintf("%s/repository/%s-artifacts/%s", artifactsRepositoryDomain, repoName, archiveFile)

	log.Info("ARTIFACT_DIR=%s", artifactDir)
	log.Info("ARTIFACT_FILE=%s", archiveFile)
	log.Info("ARTIFACTS_REPOSITORY_DOMAIN=%s", artifactsRepositoryDomain)
	log.Info("ARTIFACT_ARCHIVE_URL=%s", artifactArchiveURL)
	log.Info("URL Accessibility Code=%d", accessibilityCode)
	err = listFiles(artifactDir)
	if err != nil {
		return err
	}

	// Check if artifact file exists
	if _, err = os.Stat(artifactPath); os.IsNotExist(err) {
		return fmt.Errorf("artifact %s not found in %s. Execute 'plasmactl platform:package' before", archiveFile, artifactDir)
	}

	// Logic
	fmt.Printf("Looking for artifact %s in %s\n", archiveFile, artifactDir)
	fmt.Printf("Publishing artifact %s/%s to %s...\n", artifactDir, archiveFile, artifactArchiveURL)

	file, err := os.Open(path.Clean(artifactPath))
	if err != nil {
		log.Debug("%s", err)
		return errors.New("error opening artifact file")
	}
	defer file.Close()

	client := &http.Client{}
	req, err := http.NewRequest("PUT", artifactArchiveURL, file)
	if err != nil {
		log.Debug("%s", err)
		return errors.New("error creating HTTP request")
	}
	req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		log.Debug("%s", err)
		return errors.New("error uploading artifact")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to upload artifact: %s", resp.Status)
	}

	fmt.Println("Artifact successfully uploaded")

	return nil
}
