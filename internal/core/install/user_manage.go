package install

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type SysUser struct {
	Name  string
	UID   int
	Shell string
}

func getNonPrivilegedUsers() []SysUser {
	content, err := os.ReadFile("/etc/passwd")
	if err != nil {
		log.Printf("Warning: Failed to read /etc/passwd: %v", err)
		return nil
	}

	var users []SysUser
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 7 {
			uid, err := strconv.Atoi(parts[2])
			if err == nil && uid > 1000 {
				shell := parts[6]
				if shell == "/bin/bash" || shell == "/bin/zsh" {
					users = append(users, SysUser{
						Name:  parts[0],
						UID:   uid,
						Shell: shell,
					})
				}
			}
		}
	}
	return users
}

func promptAndManageUser() string {
	fmt.Println("\n--- User Configuration ---")
	fmt.Println("VLX_ChatBridge needs a user to run securely.")
	fmt.Println("You can create a dedicated user (default: chatbridge) or use an existing one.")

	existingUsers := getNonPrivilegedUsers()
	if len(existingUsers) > 0 {
		fmt.Println("Existing non-privileged users:")
		for i, u := range existingUsers {
			fmt.Printf("  %d. %s (uid: %d, shell: %s)\n", i+1, u.Name, u.UID, u.Shell)
		}
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter username to use [chatbridge]: ")
	input, err := reader.ReadString('\n')
	if err != nil {
		log.Fatalf("Failed to read input: %v", err)
	}

	chosenUser := strings.TrimSpace(input)
	if chosenUser == "" {
		chosenUser = "chatbridge"
	}

	userExists := false
	for _, u := range existingUsers {
		if u.Name == chosenUser {
			userExists = true
			break
		}
	}

	// Double check user existence using 'id' command as well
	if !userExists {
		err := exec.Command("id", "-u", chosenUser).Run()
		if err == nil {
			userExists = true
		}
	}

	if userExists {
		fmt.Printf("Using existing user: %s\n", chosenUser)
	} else {
		fmt.Printf("User '%s' does not exist. Creating dedicated user...\n", chosenUser)
		cmd := exec.Command("useradd", "-r", "-m", "-s", "/bin/bash", chosenUser)
		if err := cmd.Run(); err != nil {
			log.Fatalf("Failed to create user %s: %v", chosenUser, err)
		}
		fmt.Printf("Successfully created user: %s\n", chosenUser)
	}

	return chosenUser
}
