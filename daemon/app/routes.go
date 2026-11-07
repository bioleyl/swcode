package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"swcode/daemon/catalog"
	"swcode/daemon/config"
	"swcode/daemon/hosts"
	"swcode/daemon/llm"
	"swcode/daemon/protocol"
)

const (
	noPicture  = "There is no such picture."
	noCode     = "Send the pairing code of the QR code."
	codeUsed   = "This pairing code expired or was already used: scan the QR code again, it changes every minute."
	noDevice   = "There is no such phone."
	noProject  = "There is no such project."
	busy       = "A session is working: stop it first. The daemon restarts to take the new settings."
	unreadable = "The settings page sent something the daemon does not understand."
	unreached  = "Nothing answered there as llm-server."
	// keepCached: a picture's address never shows another one, it can stay in the browser's cache.
	keepCached = "private, max-age=31536000, immutable"
)

var (
	pictureTypes = map[string]string{".jpg": "image/jpeg", ".png": "image/png"}
	// pictureName and sessionName are names as the daemon makes them, so a request never leaves the pictures' folder.
	pictureName = regexp.MustCompile(`^[\w-]+\.(?:png|jpg)$`)
	sessionName = regexp.MustCompile(`^[\w-]+$`)
)

// statusRoutes are the health, stopping the daemon when it can be, and its status.
func (a *App) statusRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointHealth, func(response http.ResponseWriter, _ *http.Request) {
		answer(response, map[string]bool{"ok": true})
	})
	if a.options.OnStop != nil {
		mux.HandleFunc("POST "+protocol.EndpointStop, func(response http.ResponseWriter, _ *http.Request) {
			answer(response, map[string]bool{"ok": true})
			// The answer leaves before the daemon closes its connections.
			go a.options.OnStop()
		})
	}
	mux.HandleFunc("GET "+protocol.EndpointStatus, func(response http.ResponseWriter, request *http.Request) {
		computer, _ := os.Hostname()
		answer(response, protocol.Status{
			Computer: computer, LANURLs: a.LanURLs, Llama: a.Models.Status(request.Context()),
			PublicURL: a.config.PublicURL, Thinking: a.config.Thinking, Update: a.Updates.Offer(), Version: a.options.Version,
			Workspace: a.workspace,
		})
	})
}

// imageRoutes are the pictures the agents made and those people pasted, as the daemon keeps them for the app.
func (a *App) imageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointImages+"/{session}/{file}", func(response http.ResponseWriter, request *http.Request) {
		session, file := request.PathValue("session"), request.PathValue("file")
		kind := pictureTypes[filepath.Ext(file)]
		if !sessionName.MatchString(session) || !pictureName.MatchString(file) || kind == "" {
			fail(response, http.StatusNotFound, noPicture)
			return
		}
		data, err := os.ReadFile(filepath.Join(a.imagesFolder, session, file))
		if err != nil {
			fail(response, http.StatusNotFound, noPicture)
			return
		}
		response.Header().Set("content-type", kind)
		response.Header().Set("cache-control", keepCached)
		_, _ = response.Write(data)
	})
}

// pairingRoutes pair a phone: the computer asks for a code to show (behind the token, like every route), the phone
// trades it for its own token (open: the code is the secret), and the paired phones can be listed and forgotten.
func (a *App) pairingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointPairingCode, func(response http.ResponseWriter, _ *http.Request) {
		answer(response, a.pairing.Issue())
	})
	mux.HandleFunc("POST "+protocol.EndpointPairing, func(response http.ResponseWriter, request *http.Request) {
		var asked protocol.PairRequest
		if readBody(response, request, &asked) != nil || asked.Code == "" {
			fail(response, http.StatusBadRequest, noCode)
			return
		}
		token := a.pairing.Redeem(asked.Code, asked.Name)
		if token == "" {
			fail(response, http.StatusForbidden, codeUsed)
			return
		}
		answer(response, protocol.PairAnswer{Token: token})
	})
	mux.HandleFunc("GET "+protocol.EndpointDevices, func(response http.ResponseWriter, _ *http.Request) {
		answer(response, protocol.Devices{Devices: a.pairing.Devices()})
	})
	mux.HandleFunc("DELETE "+protocol.EndpointDevices+"/{id}", func(response http.ResponseWriter, request *http.Request) {
		if !a.pairing.Forget(request.PathValue("id")) {
			fail(response, http.StatusNotFound, noDevice)
			return
		}
		answer(response, map[string]bool{"ok": true})
	})
}

// settingsRoutes are the settings page's: the settings in use, saving them (the daemon then restarts, or asks the
// person to), and the models of an llm-server before its address is saved.
func (a *App) settingsRoutes(mux *http.ServeMux) {
	file := filepath.Join(a.config.DataDir, config.SettingsFile)
	mux.HandleFunc("GET "+protocol.EndpointSettings, func(response http.ResponseWriter, _ *http.Request) {
		answer(response, protocol.SettingsView{
			Environment: a.fixedSettings(), Path: file, Restart: a.restartKind(), Values: a.settingValues(file),
		})
	})
	mux.HandleFunc("PUT "+protocol.EndpointSettings, func(response http.ResponseWriter, request *http.Request) {
		saved := a.saveSettings(response, request, file)
		status := http.StatusOK
		if _, refused := saved.Value.(protocol.SettingsSavedFailed); refused {
			status = http.StatusBadRequest
		}
		writeJSON(response, status, saved)
	})
	mux.HandleFunc("POST "+protocol.EndpointSettingsModels, func(response http.ResponseWriter, request *http.Request) {
		var query protocol.ModelsQuery
		if readBody(response, request, &query) != nil {
			answer(response, protocol.ModelsAnswer{Value: protocol.ModelsAnswerFailed{Problem: unreadable}})
			return
		}
		served, reachable := llm.NewStatusReader(llm.Options{APIKey: query.APIKey, HTTP: a.http, URL: query.URL}).Models(request.Context())
		if !reachable {
			answer(response, protocol.ModelsAnswer{Value: protocol.ModelsAnswerFailed{Problem: unreached}})
			return
		}
		answer(response, protocol.ModelsAnswer{Value: protocol.ModelsAnswerOk{Models: served}})
	})
}

// modelRoutes are the llm-servers' and the presets': as they are, with the servers' keys, and all of them saved
// together, in use at once.
func (a *App) modelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointModels, func(response http.ResponseWriter, _ *http.Request) {
		answer(response, a.Models.Settings())
	})
	mux.HandleFunc("PUT "+protocol.EndpointModels, func(response http.ResponseWriter, request *http.Request) {
		var asked protocol.ModelSettings
		if err := readBody(response, request, &asked); err != nil {
			writeJSON(response, http.StatusBadRequest, protocol.ModelsSaved{Value: protocol.ModelsSavedFailed{Problem: err.Error()}})
			return
		}
		if problem := a.Models.Save(asked); problem != "" {
			writeJSON(response, http.StatusBadRequest, protocol.ModelsSaved{Value: protocol.ModelsSavedFailed{Problem: problem}})
			return
		}
		answer(response, protocol.ModelsSaved{Value: protocol.ModelsSavedOk{}})
	})
}

func (a *App) restartKind() protocol.Restart { return restartOf(a.options.OnRestart) }

// restartOf is how a change applies: the daemon restarts by itself when it has a way, or the person restarts it.
func restartOf(restart func()) protocol.Restart {
	if restart == nil {
		return protocol.RestartPerson
	}
	return protocol.RestartSelf
}

// fixedSettings are the settings set in the environment, which comes on top of the file: the page cannot change
// them.
func (a *App) fixedSettings() []protocol.SettingName {
	fixed := []protocol.SettingName{}
	for _, name := range protocol.EditableSettings {
		if strings.TrimSpace(a.options.Environment[string(name)]) != "" {
			fixed = append(fixed, name)
		}
	}
	return fixed
}

// settingValues is the settings in use: the file's, the environment's on top.
func (a *App) settingValues(file string) map[protocol.SettingName]string {
	merged := config.ReadSettings(file)
	for name, value := range a.options.Environment {
		merged[name] = value
	}
	values := map[protocol.SettingName]string{}
	for _, name := range protocol.EditableSettings {
		if value := merged[string(name)]; strings.TrimSpace(value) != "" {
			values[name] = value
		}
	}
	return values
}

func savedProblem(problem string) protocol.SettingsSaved {
	return protocol.SettingsSaved{Value: protocol.SettingsSavedFailed{Problem: problem}}
}

func (a *App) saveSettings(response http.ResponseWriter, request *http.Request, file string) protocol.SettingsSaved {
	var asked map[protocol.SettingName]string
	if readBody(response, request, &asked) != nil {
		return savedProblem(unreadable)
	}
	for name := range asked {
		if !name.Valid() {
			return savedProblem(unreadable)
		}
	}
	if a.options.Busy != nil && a.options.Busy() {
		return savedProblem(busy)
	}
	// The page sends what it changes; a setting it leaves out (set in the environment) keeps its value in the file.
	values := config.ReadSettings(file)
	for name, value := range asked {
		values[string(name)] = value
	}
	if problem := a.settingsProblem(values); problem != "" {
		return savedProblem(problem)
	}
	if err := config.WriteSettings(file, values); err != nil {
		return savedProblem(err.Error())
	}
	if a.options.OnRestart == nil {
		return protocol.SettingsSaved{Value: protocol.SettingsSavedOk{Restart: protocol.RestartPerson}}
	}
	// The answer leaves before the daemon closes.
	go a.options.OnRestart()
	return protocol.SettingsSaved{Value: protocol.SettingsSavedOk{Restart: protocol.RestartSelf}}
}

// settingsProblem is why these settings would not start a daemon, if they would not.
func (a *App) settingsProblem(values map[string]string) string {
	merged := map[string]string{}
	for name, value := range values {
		merged[name] = value
	}
	for name, value := range a.options.Environment {
		merged[name] = value
	}
	if _, err := config.Load(merged, a.options.Defaults); err != nil {
		return err.Error()
	}
	return ""
}

// skillRoutes are a project's page's: the skills its sessions find, read on its host as a turn reads them, and the
// swcode-base skill, which the skills CLI installs in the home or in the project, on the project's host, or takes out
// of either.
func (a *App) skillRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointSkills, func(response http.ResponseWriter, request *http.Request) {
		project, found := a.Projects.ByID(request.URL.Query().Get(protocol.ProjectParam))
		if !found {
			fail(response, http.StatusNotFound, noProject)
			return
		}
		answer(response, a.projectSkills(request.Context(), project))
	})
	mux.HandleFunc("GET "+protocol.EndpointSwcodeBase, func(response http.ResponseWriter, request *http.Request) {
		answer(response, a.SwcodeBase.Status(request.Context(), a.projectOf(request)))
	})
	mux.HandleFunc("POST "+protocol.EndpointSwcodeBase, func(response http.ResponseWriter, request *http.Request) {
		answer(response, a.SwcodeBase.Install(request.Context(), a.projectOf(request), false))
	})
	mux.HandleFunc("DELETE "+protocol.EndpointSwcodeBase, func(response http.ResponseWriter, request *http.Request) {
		answer(response, a.SwcodeBase.Remove(request.Context(), a.projectOf(request), false))
	})
	mux.HandleFunc("PUT "+protocol.EndpointSwcodeBaseHome, func(response http.ResponseWriter, request *http.Request) {
		answer(response, a.SwcodeBase.Install(request.Context(), a.projectOf(request), true))
	})
	mux.HandleFunc("DELETE "+protocol.EndpointSwcodeBaseHome, func(response http.ResponseWriter, request *http.Request) {
		answer(response, a.SwcodeBase.Remove(request.Context(), a.projectOf(request), true))
	})
}

// projectOf is the project the request names, or nothing: without one, swcode-base answers for this computer's home
// alone.
func (a *App) projectOf(request *http.Request) *protocol.Project {
	project, found := a.Projects.ByID(request.URL.Query().Get(protocol.ProjectParam))
	if !found {
		return nil
	}
	return &project
}

// projectSkills is each name once, the person's first, whether the project turned it off or not; why the host could
// not be read, when it could not.
func (a *App) projectSkills(ctx context.Context, project protocol.Project) protocol.ProjectSkills {
	found, err := catalog.Find(ctx, a.hosts.HostOf(project.Host), project.Path)
	if err != nil {
		return protocol.ProjectSkills{Problem: fmt.Sprintf("%s could not be read: %v", hosts.Label(project.Host), err)}
	}
	listed := []protocol.ProjectSkill{}
	for _, skill := range catalog.Distinct(found) {
		listed = append(listed, protocol.ProjectSkill{Description: skill.Description, Folder: skill.Folder, Name: skill.Name, Origin: skill.Origin})
	}
	return protocol.ProjectSkills{Skills: listed}
}

// closingRoutes tell what closing the desktop app would stop, for its window to ask first: no warning when nothing
// runs.
func (a *App) closingRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+protocol.EndpointClosing, func(response http.ResponseWriter, _ *http.Request) {
		work := RunningWorkOf(a.Sessions.List())
		if work.Idle() {
			answer(response, map[string]any{"idle": true, "warning": ""})
			return
		}
		answer(response, map[string]any{"idle": false, "warning": work.Warning()})
	})
}
