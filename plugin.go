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
)

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin is [launchr.Plugin] providing publish action.
type Plugin struct {
	k keyring.Keyring
}

// PluginInfo implements [launchr.Plugin] interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{
		Weight: 10,
	}
}

// OnAppInit implements [launchr.OnAppInitPlugin] interface.
func (p *Plugin) OnAppInit(app launchr.App) error {
	app.GetService(&p.k)
	return nil
}

// CobraAddCommands implements [launchr.CobraPlugin] interface to provide bump functionality.
func (p *Plugin) CobraAddCommands(rootCmd *launchr.Command) error {
	var username string
	var password string

	var pblCmd = &launchr.Command{
		Use:   "publish",
		Short: "Upload local artifact archive to private repository",
		RunE: func(cmd *launchr.Command, _ []string) error {
			// Don't show usage help on a runtime error.
			cmd.SilenceUsage = true

			return publish(username, password, p.k)
		},
	}

	pblCmd.Flags().StringVarP(&username, "username", "", "", "Username for artifact repository")
	pblCmd.Flags().StringVarP(&password, "password", "", "", "Password for artifact repository")

	rootCmd.AddCommand(pblCmd)
	return nil
}

func publish(username, password string, k keyring.Keyring) error {
	// Get repository information
	repoName, lastCommitShortSHA, err := getRepoInfo()
	if err != nil {
		launchr.Log().Error("error", "error", err)
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

	launchr.Log().Info("artifact info",
		"ARTIFACT_DIR", artifactDir,
		"ARTIFACT_FILE", archiveFile,
		"ARTIFACTS_REPOSITORY_DOMAIN", artifactsRepositoryDomain,
		"ARTIFACT_ARCHIVE_URL", artifactArchiveURL,
		"URL Accessibility Code", accessibilityCode,
	)
	err = listFiles(artifactDir)
	if err != nil {
		return err
	}

	// Check if artifact file exists
	if _, err = os.Stat(artifactPath); os.IsNotExist(err) {
		return fmt.Errorf("artifact %s not found in %s. Execute 'plasmactl package' before", archiveFile, artifactDir)
	}

	launchr.Term().Printfln("Looking for artifact %s in %s", archiveFile, artifactDir)
	file, err := os.Open(path.Clean(artifactPath))
	if err != nil {
		launchr.Log().Error("error", "error", err)
		return errors.New("error opening artifact file")
	}
	defer file.Close()

	client := &http.Client{}

	launchr.Term().Println("Getting credentials")
	ci, save, err := getCredentials(artifactsRepositoryDomain, username, password, k)
	if err != nil {
		return err
	}

	authRequest, err := http.NewRequest(http.MethodHead, artifactsRepositoryDomain, http.NoBody)
	if err != nil {
		launchr.Log().Error("error", "error", err)
		return errors.New("error creating HTTP request")
	}

	authRequest.SetBasicAuth(ci.Username, ci.Password)
	respAuth, err := client.Do(authRequest)
	if respAuth.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to upload artifact: %s", respAuth.Status)
	}

	uploadRequest, err := http.NewRequest("PUT", artifactArchiveURL, file)
	if err != nil {
		return err
	}
	uploadRequest.SetBasicAuth(ci.Username, ci.Password)

	launchr.Term().Printfln("Publishing artifact %s/%s to %s...", artifactDir, archiveFile, artifactArchiveURL)
	respUpload, err := client.Do(uploadRequest)
	if err != nil {
		launchr.Log().Error("error", "error", err)
		return errors.New("error uploading artifact")
	}
	defer respUpload.Body.Close()

	if respUpload.StatusCode != http.StatusOK && respUpload.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to upload artifact: %s", respUpload.Status)
	}

	launchr.Term().Success().Println("Artifact successfully uploaded")

	defer func() {
		if save {
			err = k.Save()
			if err != nil {
				launchr.Log().Error("error during saving keyring file", "error", err)
			}
		}
	}()

	return nil
}

func getCredentials(url, username, password string, k keyring.Keyring) (keyring.CredentialsItem, bool, error) {
	ci, err := k.GetForURL(url)
	save := false
	if err != nil {
		if errors.Is(err, keyring.ErrEmptyPass) {
			return ci, false, err
		} else if !errors.Is(err, keyring.ErrNotFound) {
			launchr.Log().Error("error", "error", err)
			return ci, false, errors.New("the keyring is malformed or wrong passphrase provided")
		}
		ci = keyring.CredentialsItem{}
		ci.URL = url
		ci.Username = username
		ci.Password = password
		if ci.Username == "" || ci.Password == "" {
			if ci.URL != "" {
				launchr.Term().Info().Printfln("Please add login and password for URL - %s", ci.URL)
			}
			err = keyring.RequestCredentialsFromTty(&ci)
			if err != nil {
				return ci, false, err
			}
		}

		err = k.AddItem(ci)
		if err != nil {
			return ci, false, err
		}

		save = true
	}

	return ci, save, nil
}
