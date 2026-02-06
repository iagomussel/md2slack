package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func runInstall() {
	fmt.Println("Installing in user mode (no sudo required)...")

	// 1. Link Project Directory to ~/.md2slack
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting user home directory: %v\n", err)
		return
	}

	targetLink := filepath.Join(home, ".md2slack")

	// Use current directory as the source for the link
	absCwd, err := filepath.Abs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error determining current directory: %v\n", err)
		return
	}

	// Remove existing symlink or directory
	if stats, err := os.Lstat(targetLink); err == nil {
		if stats.Mode()&os.ModeSymlink != 0 {
			fmt.Println("Removing existing ~/.md2slack symlink...")
			os.Remove(targetLink)
		} else if stats.IsDir() {
			fmt.Println("Backing up existing ~/.md2slack directory to ~/.md2slack.bak")
			os.Rename(targetLink, targetLink+".bak")
		}
	}

	if err := os.Symlink(absCwd, targetLink); err != nil {
		fmt.Fprintf(os.Stderr, "Error linking ~/.md2slack: %v\n", err)
		return
	} else {
		fmt.Printf("Linked ~/.md2slack -> %s\n", absCwd)
	}

	// 2. Advise on PATH
	fmt.Printf("\nInstallation successful!\n")
	fmt.Printf("Please add the following to your ~/.bashrc or ~/.zshrc:\n\n")
	fmt.Printf("export PATH=$PATH:$HOME/.md2slack\n\n")
	fmt.Printf("Then run: source ~/.bashrc\n")
}
