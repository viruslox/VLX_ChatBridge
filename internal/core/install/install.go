package install

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

func Run() {
	fmt.Println("Running installer...")

	// 1. Root Check
	if os.Geteuid() != 0 {
		log.Fatal("Error: The install command must be run as root (or using sudo).")
	}

	// 2. Directory Creation
	baseDir := "/opt/VLX_ChatBridge"
	binDir := filepath.Join(baseDir, "bin")
	etcDir := filepath.Join(baseDir, "etc")

	dirs := []string{baseDir, binDir, etcDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}
	fmt.Println("Created installation directories in", baseDir)

	// 3. Binary Copying
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path: %v", err)
	}

	destExePath := filepath.Join(binDir, "VLX_ChatBridge")
	if err := copyFile(exePath, destExePath, 0755); err != nil {
		log.Fatalf("Failed to copy executable to %s: %v", destExePath, err)
	}
	fmt.Println("Copied executable to", destExePath)

	// 4. Configuration Merging
	handleConfigurationFiles(etcDir)

	// 5. User Prompt & Management
	chosenUser := promptAndManageUser()

	// 6. Finalize Settings & Ownership
	finalizeInstallation(etcDir, baseDir, chosenUser)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}

// configuration merging logic
