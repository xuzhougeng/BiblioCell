package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

func main() {
	if len(os.Args) > 1 && isCLICommand(os.Args[1]) {
		os.Exit(runCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}

	libraryService := &Library{}
	mygo.Bind(libraryService)

	var store *library.Store
	var mcpServer *library.MCPServer
	var stopUpdates func()
	mygo.App.WhenReady(func() {
		dataDir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatalf("locate application data directory: %v", err)
		}
		settingsStore, err := openSettings(dataDir)
		libraryService.settings = settingsStore
		if err != nil {
			log.Printf("Using default settings: %v", err)
		}
		store, err = library.Open(settingsStore.Get().DatabasePath())
		if err != nil {
			log.Fatalf("initialize BiblioCell: %v", err)
		}
		libraryService.store = store

		configuredToken := settingsStore.Get().MCPToken
		if configuredToken == "" {
			log.Printf("MCP authentication unavailable: no bearer token")
		} else {
			mcpServer, err = library.StartMCP(store, library.MCPAddress, configuredToken)
			if err != nil {
				log.Printf("MCP server unavailable on %s: %v", library.MCPAddress, err)
			} else {
				libraryService.mcpEndpoint = mcpServer.Endpoint
				libraryService.mcpToken = configuredToken
				log.Printf("MCP server listening on %s (read-only without bearer token)", mcpServer.Endpoint)
			}
		}

		stopUpdates = store.StartSourceUpdates(sourceCredentials(settingsStore), func() {
			automation, err := store.SourceAutomation(context.Background())
			if err != nil {
				log.Printf("read source automation: %v", err)
				return
			}
			if err := sourcesChanged.Broadcast(automation); err != nil {
				log.Printf("announce source update: %v", err)
			}
		})

		mygo.NewWindow(mygo.WindowOptions{
			Title:           "BiblioCell",
			Width:           1440,
			Height:          900,
			MinWidth:        960,
			MinHeight:       600,
			BackgroundColor: "#f7f8f5",
			StateKey:        "main",
			URL:             "/",
		})
	})

	// OnQuit, not after Run: a relaunch starts the next instance before Run
	// returns, and that instance needs the MCP port.
	mygo.App.OnQuit(func() {
		if stopUpdates != nil {
			stopUpdates() // Before the store closes: a run may be writing its result.
		}
		if mcpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := mcpServer.Close(ctx); err != nil {
				log.Printf("close MCP server: %v", err)
			}
			cancel()
		}
		if store != nil {
			if err := store.Close(); err != nil {
				log.Printf("close database: %v", err)
			}
		}
	})

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// sourceCredentials reads the saved keys at each use, so edits need no restart.
func sourceCredentials(store *settings.Store) func() library.SourceCredentials {
	return func() library.SourceCredentials {
		config := store.Get()
		return library.SourceCredentials{NCBIAPIKey: config.NCBIAPIKey, CrossrefEmail: config.CrossrefEmail}
	}
}

// openSettings migrates legacy data under dataDir and loads settings.json.
// The returned store is always usable: on a missing or invalid file it holds
// defaults, and the error says why.
func openSettings(dataDir string) (*settings.Store, error) {
	migrateLegacyData(dataDir)
	migrateDatabaseSetting(filepath.Join(dataDir, "settings.json"))
	mcpToken, err := library.LoadOrCreateMCPToken(dataDir)
	if err != nil {
		log.Printf("MCP token file unavailable: %v", err)
	}
	return settings.Open(
		filepath.Join(dataDir, "settings.json"),
		dataDir,
		mcpToken,
	)
}

// Names of the data files written before the BiblioCell rename.
const (
	legacyAppName      = "PlantCell Library"
	legacyDatabaseName = "plantcell.sqlite"
	legacySettingsName = "settings.json"
	legacyTokenName    = "mcp-token"
)

// migrateLegacyData carries data from the previously named PlantCell Library
// data directory into the BiblioCell data directory. It moves missing files
// once and rewrites a recorded database path that still points into the
// legacy directory; a database at a custom location keeps its original path.
func migrateLegacyData(dataDir string) {
	if configDir, err := os.UserConfigDir(); err == nil {
		migrateLegacyDir(filepath.Join(configDir, legacyAppName), dataDir)
	}
}

// migrateLegacyDir moves the legacy data into dataDir. It keeps both
// directory names as arguments so tests can exercise real directories.
func migrateLegacyDir(legacyDir, dataDir string) {
	if legacyDir == dataDir {
		return
	}
	legacyDB := filepath.Join(legacyDir, legacyDatabaseName)
	if _, err := os.Stat(legacyDB); err != nil {
		return
	}
	// Move the whole legacy directory while the new location is unused.
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if err := os.Rename(legacyDir, dataDir); err != nil {
			log.Printf("move legacy data directory: %v", err)
		} else {
			log.Printf("moved legacy data directory into %s", dataDir)
		}
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Printf("prepare data directory: %v", err)
		return
	}
	// Carry the database by file when both directories exist.
	newDB := filepath.Join(dataDir, "bibliocell.sqlite")
	if _, err := os.Stat(newDB); os.IsNotExist(err) {
		for _, dir := range []string{legacyDir, dataDir} {
			old := filepath.Join(dir, legacyDatabaseName)
			if _, err := os.Stat(old); err != nil {
				continue
			}
			if err := os.Rename(old, newDB); err != nil {
				log.Printf("move legacy database: %v", err)
			} else {
				log.Printf("moved legacy database into %s", newDB)
			}
			break
		}
	}
	// Carry settings and the MCP token so preferences survive the rename.
	for _, name := range []string{legacySettingsName, legacyTokenName} {
		to := filepath.Join(dataDir, name)
		if _, err := os.Stat(to); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(legacyDir, name))
		if err != nil {
			continue
		}
		if err := os.WriteFile(to, data, 0o600); err != nil {
			log.Printf("carry legacy %s: %v", name, err)
		}
	}
	fixLegacyDatabasePath(filepath.Join(dataDir, legacySettingsName), legacyDir, dataDir)
}

// fixLegacyDatabasePath rewrites a recorded database path that still points
// into the legacy directory, and renames the default database file in place.
// Paths outside the data directories are left untouched.
func fixLegacyDatabasePath(settingsPath, legacyDir, dataDir string) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return
	}
	recorded := filepath.Clean(strings.TrimSpace(configString(config, "databasePath")))
	if recorded == "" || recorded == "." {
		return
	}
	updated := recorded
	if recorded == legacyDir || strings.HasPrefix(recorded, legacyDir+string(os.PathSeparator)) {
		updated = filepath.Join(dataDir, strings.TrimPrefix(strings.TrimPrefix(recorded, legacyDir), string(os.PathSeparator)))
	}
	if filepath.Dir(updated) == dataDir && filepath.Base(updated) == legacyDatabaseName {
		updated = filepath.Join(dataDir, "bibliocell.sqlite")
	}
	if updated == recorded {
		return
	}
	config["databasePath"] = updated
	if err := writeConfig(settingsPath, config); err != nil {
		log.Printf("update settings database path: %v", err)
	} else {
		log.Printf("database path updated to %s", updated)
	}
}

// migrateDatabaseSetting replaces the databasePath setting of older versions
// with the folder holding the database. The file name is now fixed, so a
// custom-named database is renamed inside its folder; an existing
// bibliocell.sqlite is never overwritten.
func migrateDatabaseSetting(settingsPath string) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return
	}
	recorded := filepath.Clean(strings.TrimSpace(configString(config, "databasePath")))
	if recorded == "." {
		return
	}
	folder := filepath.Dir(recorded)
	target := filepath.Join(folder, settings.DatabaseFile)
	if _, err := os.Stat(recorded); err == nil && recorded != target {
		if _, err := os.Stat(target); err == nil {
			log.Printf("keep databasePath setting: %s already exists", target)
			return
		}
		// The journal moves first so a failed rename can retry on the next start.
		for _, suffix := range []string{"-journal", ""} {
			if err := os.Rename(recorded+suffix, target+suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("rename database: %v", err)
				return
			}
		}
	}
	delete(config, "databasePath")
	config["storagePath"] = folder
	if err := writeConfig(settingsPath, config); err != nil {
		log.Printf("update settings storage path: %v", err)
	}
}

// writeConfig replaces settings.json with decoded settings.
func writeConfig(path string, config map[string]any) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// configString reads a string field from decoded JSON settings.
func configString(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return value
}
