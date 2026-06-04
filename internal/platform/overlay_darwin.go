//go:build darwin

package platform

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const swiftOverlaySource = `
import AppKit

class OverlayPanel: NSPanel {
    override var canBecomeKey: Bool { false }
    override var canBecomeMain: Bool { false }
}

class OverlayView: NSView {
    var elapsed: Int = 0
    var duration: Int = 0
    var isCountdown: Bool = false
    var timer: Timer?
    var dotOpacity: CGFloat = 1.0
    var dotFadingOut = true

    func start(mode: String, dur: Int) {
        isCountdown = (mode == "countdown")
        duration = dur
        elapsed = 0

        timer = Timer.scheduledTimer(withTimeInterval: 1.0/30.0, repeats: true) { [weak self] _ in
            guard let self = self else { return }
            // Update dot pulse
            if self.dotFadingOut {
                self.dotOpacity -= 0.02
                if self.dotOpacity <= 0.3 { self.dotFadingOut = false }
            } else {
                self.dotOpacity += 0.02
                if self.dotOpacity >= 1.0 { self.dotFadingOut = true }
            }
            self.needsDisplay = true
        }

        // Separate 1-second timer for time updates
        Timer.scheduledTimer(withTimeInterval: 1.0, repeats: true) { [weak self] _ in
            guard let self = self else { return }
            self.elapsed += 1
            if self.isCountdown && self.elapsed >= self.duration {
                NSApplication.shared.terminate(nil)
            }
        }
    }

    override func draw(_ dirtyRect: NSRect) {
        // Background
        NSColor(white: 0.0, alpha: 0.70).setFill()
        let bg = NSBezierPath(roundedRect: bounds, xRadius: 8, yRadius: 8)
        bg.fill()

        // Red dot
        let dotRect = NSRect(x: 12, y: bounds.height/2 - 5, width: 10, height: 10)
        NSColor(red: 1.0, green: 0.2, blue: 0.2, alpha: dotOpacity).setFill()
        NSBezierPath(ovalIn: dotRect).fill()

        // "REC" label
        let recAttrs: [NSAttributedString.Key: Any] = [
            .font: NSFont.monospacedSystemFont(ofSize: 13, weight: .bold),
            .foregroundColor: NSColor.white
        ]
        let recStr = NSAttributedString(string: "REC", attributes: recAttrs)
        recStr.draw(at: NSPoint(x: 28, y: bounds.height/2 - 8))

        // Time
        let seconds: Int
        if isCountdown {
            seconds = max(duration - elapsed, 0)
        } else {
            seconds = elapsed
        }
        let timeStr = String(format: "%02d:%02d", seconds / 60, seconds % 60)
        let timeAttrs: [NSAttributedString.Key: Any] = [
            .font: NSFont.monospacedDigitSystemFont(ofSize: 13, weight: .medium),
            .foregroundColor: NSColor(white: 0.9, alpha: 1.0)
        ]
        let timeNS = NSAttributedString(string: timeStr, attributes: timeAttrs)
        timeNS.draw(at: NSPoint(x: 68, y: bounds.height/2 - 8))
    }
}

// Parse args
var mode = "elapsed"
var duration = 0
let args = CommandLine.arguments
var i = 1
while i < args.count {
    if args[i] == "--mode" && i+1 < args.count {
        mode = args[i+1]; i += 2
    } else if args[i] == "--duration" && i+1 < args.count {
        duration = Int(args[i+1]) ?? 0; i += 2
    } else {
        i += 1
    }
}

let app = NSApplication.shared
app.setActivationPolicy(.accessory)

let panelWidth: CGFloat = 130
let panelHeight: CGFloat = 36
guard let screen = NSScreen.main else { exit(1) }
let screenFrame = screen.visibleFrame
let x = screenFrame.maxX - panelWidth - 16
let y = screenFrame.maxY - panelHeight - 16

let panel = OverlayPanel(
    contentRect: NSRect(x: x, y: y, width: panelWidth, height: panelHeight),
    styleMask: [.nonactivatingPanel, .borderless],
    backing: .buffered,
    defer: false
)
panel.level = .floating
panel.isOpaque = false
panel.backgroundColor = .clear
panel.hasShadow = true
panel.ignoresMouseEvents = true
panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]

let overlayView = OverlayView(frame: NSRect(x: 0, y: 0, width: panelWidth, height: panelHeight))
panel.contentView = overlayView
panel.orderFrontRegardless()

overlayView.start(mode: mode, dur: duration)

signal(SIGTERM, SIG_IGN)
let termSource = DispatchSource.makeSignalSource(signal: SIGTERM, queue: .main)
termSource.setEventHandler { NSApplication.shared.terminate(nil) }
termSource.resume()

app.run()
`

// OverlayProcess holds a running overlay subprocess.
type OverlayProcess struct {
	cmd *exec.Cmd
}

// HasSwift returns true if swiftc is available.
func HasSwift() bool {
	_, err := exec.LookPath("swiftc")
	return err == nil
}

// overlayDir returns the directory for cached overlay binary.
func overlayDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "nikte", "overlay"), nil
}

// ensureOverlayBinary compiles the Swift overlay if needed, returns path to binary.
func ensureOverlayBinary() (string, error) {
	dir, err := overlayDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	binaryPath := filepath.Join(dir, "oio-overlay")
	hashPath := filepath.Join(dir, "source.sha256")

	// Check if recompilation needed
	currentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(swiftOverlaySource)))
	if existing, err := os.ReadFile(hashPath); err == nil {
		if string(existing) == currentHash {
			if _, err := os.Stat(binaryPath); err == nil {
				return binaryPath, nil // cached binary is up to date
			}
		}
	}

	// Write source and compile
	srcPath := filepath.Join(dir, "overlay.swift")
	if err := os.WriteFile(srcPath, []byte(swiftOverlaySource), 0644); err != nil {
		return "", err
	}

	cmd := exec.Command("swiftc", "-O", "-o", binaryPath, srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("swift compilation failed: %s: %w", string(out), err)
	}

	// Save hash
	os.WriteFile(hashPath, []byte(currentHash), 0644)
	os.Remove(srcPath)

	return binaryPath, nil
}

// StartOverlay launches the floating overlay indicator.
// Returns nil if overlay cannot be started (graceful degradation).
func StartOverlay(mode string, duration int) *OverlayProcess {
	if !HasSwift() {
		return nil
	}

	binaryPath, err := ensureOverlayBinary()
	if err != nil {
		return nil
	}

	args := []string{"--mode", mode}
	if duration > 0 {
		args = append(args, "--duration", fmt.Sprintf("%d", duration))
	}

	cmd := exec.Command(binaryPath, args...)
	if err := cmd.Start(); err != nil {
		return nil
	}

	return &OverlayProcess{cmd: cmd}
}

// Stop terminates the overlay process.
func (o *OverlayProcess) Stop() {
	if o == nil || o.cmd == nil || o.cmd.Process == nil {
		return
	}
	o.cmd.Process.Signal(syscall.SIGTERM)
	o.cmd.Wait()
}
