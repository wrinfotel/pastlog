package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wrinfotel/pastlog/internal/agentlog"
	"github.com/wrinfotel/pastlog/internal/mask"
	"github.com/wrinfotel/pastlog/internal/render"
	"github.com/wrinfotel/pastlog/internal/search"
)

// SessionExport reports an ExportSession call: written, or the ambiguous
// picker data.
type SessionExport struct {
	Status     string               `json:"status"` // "written" | "ambiguous"
	Path       string               `json:"path,omitempty"`
	Bytes      int                  `json:"bytes,omitempty"`
	Candidates []render.SessionJSON `json:"candidates,omitempty"`
}

// ExportSession writes one session as CLI-identical JSON or markdown
// (R-D13): the bytes come from the very render functions `pastlog show
// --json` / `--export md` use. Writes only to the user-chosen destPath.
func (a *App) ExportSession(idPrefix, format, destPath string) (SessionExport, error) {
	if err := validateExport(format, destPath); err != nil {
		return SessionExport{}, err
	}
	_, reg, err := a.registry()
	if err != nil {
		return SessionExport{}, err
	}
	adapters := reg.Adapters()
	res, err := agentlog.ResolveSession(adapters, idPrefix)
	if !res.Found() {
		if len(res.Candidates) > 0 {
			return SessionExport{Status: "ambiguous", Candidates: render.SessionRowsJSON(res.Candidates)}, nil
		}
		return SessionExport{}, err
	}

	var entries []agentlog.Entry
	if err := res.Adapter.Entries(res.Meta.Session, func(e agentlog.Entry) error {
		entries = append(entries, e)
		return nil
	}); err != nil {
		return SessionExport{}, fmt.Errorf("cannot read session %s: %v", res.Meta.ID, err)
	}
	meta := res.Meta
	var buf bytes.Buffer
	if format == "json" {
		// json stays CLI-identical (R-D13): verbatim, like `show --json`.
		if err := render.ShowJSON(&buf, meta, entries); err != nil {
			return SessionExport{}, err
		}
	} else {
		// markdown is a human artifact: masked by default like the CLI's
		// `--export md`, verbatim when the user turned masking off.
		if a.masking() {
			meta.Title = mask.Mask(meta.Title)
			for i := range entries {
				entries[i].Text = mask.Mask(entries[i].Text)
			}
		}
		render.ShowMarkdown(&buf, meta, entries)
	}
	if err := os.WriteFile(destPath, buf.Bytes(), 0o644); err != nil {
		return SessionExport{}, fmt.Errorf("cannot write the export: %v", err)
	}
	return SessionExport{Status: "written", Path: destPath, Bytes: buf.Len()}, nil
}

// ExportSessions writes the filtered sessions list as CLI-identical JSON
// (`pastlog sessions --json` bytes).
func (a *App) ExportSessions(f FilterOptions, destPath string) error {
	_, err := a.exportList(f, destPath, "sessions")
	return err
}

// ExportSearch writes the query's results using the CLI's verbatim JSON schema.
func (a *App) ExportSearch(query string, o SearchOptions, destPath string) error {
	if err := validateExport("json", destPath); err != nil {
		return err
	}
	adapters, matcher, opts, err := a.prepareSearch(query, o)
	if err != nil {
		return err
	}
	results := search.Run(adapters, matcher, opts)
	file, err := os.CreateTemp(filepath.Dir(destPath), ".pastlog-search-*.json")
	if err != nil {
		return fmt.Errorf("cannot write the export: %v", err)
	}
	defer os.Remove(file.Name()) //nolint:errcheck // Best-effort cleanup after writing or renaming.
	writeErr := render.SearchJSON(file, results)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("cannot write the export: %v", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("cannot close the export: %v", closeErr)
	}
	if err := os.Rename(file.Name(), destPath); err != nil {
		return fmt.Errorf("cannot save the export: %v", err)
	}
	return nil
}

// exportList re-runs a listing server-side and writes it; kind picks the
// renderer. Returns the rendered byte count for callers that report it.
func (a *App) exportList(f FilterOptions, destPath, kind string) (int, error) {
	if destPath == "" {
		return 0, fmt.Errorf("empty export path")
	}
	if !filepath.IsAbs(destPath) {
		return 0, fmt.Errorf("export path must be absolute: %s", destPath)
	}
	_, reg, err := a.registry()
	if err != nil {
		return 0, err
	}
	if err := validateAgent(reg, f.Agent); err != nil {
		return 0, err
	}
	filter, err := a.buildFilter(f)
	if err != nil {
		return 0, err
	}
	adapters := reg.Adapters()
	var buf bytes.Buffer
	switch kind {
	case "sessions":
		rows := agentlog.CollectSessions(adapters, filter, nil)
		err = render.SessionsJSON(&buf, rows)
	default:
		return 0, fmt.Errorf("unsupported export kind %q", kind)
	}
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(destPath, buf.Bytes(), 0o644); err != nil {
		return 0, fmt.Errorf("cannot write the export: %v", err)
	}
	return buf.Len(), nil
}

// validateExport applies the export-path guards (spec §2.1: writes go only
// to a user-chosen path) and the format whitelist.
func validateExport(format, destPath string) error {
	if format != "json" && format != "md" {
		return fmt.Errorf("unsupported export format %q (only json, md)", format)
	}
	if destPath == "" {
		return fmt.Errorf("empty export path")
	}
	if !filepath.IsAbs(destPath) {
		return fmt.Errorf("export path must be absolute: %s", destPath)
	}
	return nil
}
