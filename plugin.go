// Package plasmactlpublish implements a publish launchr plugin
package plasmactlpublish

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/log"
	"github.com/spf13/cobra"
)

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin is launchr plugin providing bump action.
type Plugin struct {
	k keyring.Keyring
}

// PluginInfo implements launchr.Plugin interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{}
}

// OnAppInit implements launchr.Plugin interface.
func (p *Plugin) OnAppInit(app launchr.App) error {
	app.GetService(&p.k)
	return nil
}

// CobraAddCommands implements launchr.CobraPlugin interface to provide bump functionality.
func (p *Plugin) CobraAddCommands(rootCmd *cobra.Command) error {
	var pblCmd = &cobra.Command{
		Use:   "publish",
		Short: "Upload local artifact archive to private repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Don't show usage help on a runtime error.
			cmd.SilenceUsage = true

			return publish(p.k)
		},
	}

	rootCmd.AddCommand(pblCmd)
	return nil
}

func publish(k keyring.Keyring) error {
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

	fmt.Printf("Looking for artifact %s in %s\n", archiveFile, artifactDir)
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

	fmt.Println("Getting credentials")
	ci, err := getCredentials(artifactsRepositoryDomain, k)
	if err != nil {
		return err
	}
	req.SetBasicAuth(ci.Username, ci.Password)

	fmt.Printf("Publishing artifact %s/%s to %s...\n", artifactDir, archiveFile, artifactArchiveURL)
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

func getCredentials(url string, k keyring.Keyring) (keyring.CredentialsItem, error) {
	ci, err := k.GetForURL(url)
	if err != nil {
		ci = keyring.CredentialsItem{}
		ci.URL = url
		if ci.URL != "" {
			fmt.Printf("Please add login and password for URL - %s\n", ci.URL)
		}
		err = keyring.RequestCredentialsFromTty(&ci)
		if err != nil {
			return ci, err
		}

		err = k.AddItem(ci)
		if err != nil {
			return ci, err
		}
		err = k.Save()
		if err != nil {
			return ci, err
		}
	}

	return ci, nil
}
