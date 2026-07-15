package mapeditor

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// pickDir opens the OS's native folder chooser and returns the chosen directory,
// or "" if the user cancelled or no chooser is installed. It blocks until the
// dialog closes, so callers run it on a goroutine (see pickAssetsDir).
func pickDir() string {
	var out []byte
	var err error
	switch runtime.GOOS {
	case "darwin":
		// #nosec G204 -- fixed AppleScript folder chooser, no external input
		out, err = exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "Choose the assets folder")`).Output()
	case "windows":
		// #nosec G204 -- fixed PowerShell folder chooser, no external input
		out, err = exec.Command("powershell", "-NoProfile", "-Command",
			`Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.FolderBrowserDialog; if($d.ShowDialog() -eq 'OK'){[Console]::Out.Write($d.SelectedPath)}`).Output()
	default:
		// #nosec G204 -- fixed zenity folder chooser, no external input
		out, err = exec.Command("zenity", "--file-selection", "--directory").Output()
	}
	if err != nil {
		return ""
	}
	return filepath.Clean(strings.TrimSpace(string(out)))
}

// pickAssetsDir launches the native folder chooser off the game loop so the window
// keeps drawing; pollDirPick applies the result. Ignored if a pick is in flight.
func (e *MapEditor) pickAssetsDir() {
	if e.dirResult != nil {
		return
	}
	ch := make(chan string, 1)
	e.dirResult = ch
	go func() { ch <- pickDir() }()
	e.status = "choose the assets folder…"
}

// pollDirPick applies a finished folder choice: it repoints the palette at the new
// directory. Called once per frame.
func (e *MapEditor) pollDirPick() {
	if e.dirResult == nil {
		return
	}
	select {
	case d := <-e.dirResult:
		e.dirResult = nil
		if d == "" || d == "." {
			e.status = "folder choice cancelled"
			return
		}
		e.assetDir = d
		e.loadPalette()
		e.status = "assets dir: " + d
	default:
	}
}
