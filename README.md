# agentstudio

The shared shell of the agent studios: desktop apps (Wails 3) where a text
language is previewed on one side and a coding agent edits it in a terminal on
the other. [Kino Studio](https://github.com/jespino/kino), agtrip, Agave and
[TCAD Studio](https://github.com/jespino/tcad-studio) are built on it.

A studio brings its language, its preview and its own API; agentstudio
brings the rest, the same everywhere:

- the agent terminal: Claude Code, Codex, Gemini CLI, Antigravity or a shell in
  the project folder, with **Resume** for agents that keep conversations, the
  studio's command first on the agent's `PATH`, and the context files agents
  read (AGENTS.md, CLAUDE.md, GEMINI.md, the skill in `.agents/skills` and
  `.claude/skills`) written before it starts;
- projects: open / new / close, recent projects, native folder dialogs, a
  folder browser for browsers, and for one-file projects the file watcher
  that pushes the agent's edits to the UI;
- the desktop shell (`app [project]`) and the same UI in a browser
  (`app serve`), with a session token, its own `Host` only and same-origin
  writes, because the API can start programs;
- the UI pieces: the backend transport (Wails or HTTP), the agent panel, the
  welcome screen, the project menu, the folder browser, a CodeMirror editor
  kept in step with the disk, and the split layout.

## Go packages

| Package | What it is |
|---|---|
| `studio` | `Base` (dialogs, recent projects, folder browser, agent terminal) and `Workspace` (a `Base` whose projects are a folder with one source file). Embed one in the studio's service struct: its methods become part of the API, for Wails and for `rpc`. |
| `terminal` | Agent harnesses in PTYs, streamed as events, with scrollback to reattach. |
| `agentctx` | Writes AGENTS.md (a managed block; the user's text around it is kept), the imports and the skill. |
| `rpc` | The service over HTTP: `POST /rpc/<Method>`, `GET /events` (SSE), the service's own routes and the frontend. |
| `browser` | `app serve`: `rpc` with the frontend and the session token. No cgo, so command-line programs can use it. |
| `desktop` | The Wails app (window, native dialogs, a project from the command line) and `serve`. |

```go
type App struct {
	*studio.Workspace // or *studio.Base for studios with their own project model
}

func New(docs Docs) *App {
	a := &App{}
	a.Workspace = studio.NewWorkspace(studio.Config{
		ID: "kino", Ext: ".kino", Default: "project.kino",
		Agent: studio.Agent{
			Context: func(dir, file string) error { return agentctx.Write(dir, kinoContext(file, docs)) },
			Env:     func(dir, file string) []string { return []string{"KINO_FILE=" + file} },
		},
	}, studio.WorkspaceOptions{Template: projectTemplate})
	return a
}

func main() {
	svc := New(docs)
	desktop.Main(svc, desktop.Options{
		ID: "kino", Name: "Kino Studio", Assets: dist,
		Routes: []string{"/__kino/"}, // the service's own endpoints (it is an http.Handler)
		Open:   func(p string) error { _, err := svc.OpenProject(p); return err },
		ServeAddr: "127.0.0.1:34116",
	})
}
```

Exported methods are the API. Those with interface, func or channel
parameters never are; others that aren't (setup, plumbing) go in
`HiddenMethods()`, which a service extends with its own.

## Frontend package

The npm package `@jespino/agentstudio` is TypeScript source (the studio's Vite
compiles it); React, xterm.js, CodeMirror, zustand, react-resizable-panels and
the Wails runtime are peer dependencies.

```ts
import { configureStudio, createBackend, workspaceApi, TerminalPanel, Welcome, ProjectPicker, ProjectMenu, StudioLayout, SourceEditor } from '@jespino/agentstudio';
import '@jespino/agentstudio/styles.css';

export const backend = createBackend({ service: 'github.com/jespino/kino/internal/app.App' });
export const api = { ...workspaceApi(backend.call), myMethod: () => backend.call('MyMethod') };

configureStudio({ backend, id: 'kino', notify: showToast, currentDir: () => store.project?.dir });
```

The components use the studio's theme variables (`--bg`, `--panel`,
`--panel-2`, `--border`, `--text`, `--text-muted`, `--accent`, `--danger`,
`--term-bg`, `--term-fg`, `--term-selection`). For `app serve` behind Vite,
`@jespino/agentstudio/vite` has the proxy entries; both sides read the session
token from `AGENTSTUDIO_TOKEN`:

```ts
import { studioProxy } from '@jespino/agentstudio/vite';
export default defineConfig({ server: { proxy: studioProxy({ backend: 'http://127.0.0.1:34116', routes: ['/__kino'] }) } });
```

## Using it

```sh
go get github.com/jespino/agentstudio@v0.1.0
npm install github:jespino/agentstudio#v0.1.0
```

To work on agentstudio and a studio together, point the studio at the local
checkout: `go mod edit -replace github.com/jespino/agentstudio=../agentstudio`
and `npm install --install-links ../agentstudio` (a copy, not a link, so there
is one React). Put the versions back before committing the studio.

## Development

```sh
go test ./...        # Go packages (the terminal tests use /bin/sh)
npm install
npm test             # the frontend (Vitest)
npm run typecheck
```

## License

MIT
