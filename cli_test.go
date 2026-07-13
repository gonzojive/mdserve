package main

import (
	"testing"
)

// TestCLI_CommandRegistration verifies that start and install commands are registered.
func TestCLI_CommandRegistration(t *testing.T) {
	cmds := rootCmd.Commands()
	foundStart := false
	foundInstall := false
	for _, cmd := range cmds {
		if cmd.Name() == "start" {
			foundStart = true
		}
		if cmd.Name() == "install" {
			foundInstall = true
		}
	}

	if !foundStart {
		t.Error("Expected 'start' subcommand to be registered")
	}
	if !foundInstall {
		t.Error("Expected 'install' subcommand to be registered")
	}
}

// TestCLI_Flags checks that persistent flags are properly registered on the root command.
func TestCLI_Flags(t *testing.T) {
	flags := rootCmd.PersistentFlags()

	portFlagDef := flags.Lookup("port")
	if portFlagDef == nil {
		t.Fatal("Expected 'port' flag to be registered")
	}
	if portFlagDef.DefValue != "8080" {
		t.Errorf("Expected 'port' default to be 8080, got %s", portFlagDef.DefValue)
	}

	dirFlagDef := flags.Lookup("dir")
	if dirFlagDef == nil {
		t.Fatal("Expected 'dir' flag to be registered")
	}
	if dirFlagDef.DefValue != "." {
		t.Errorf("Expected 'dir' default to be '.', got %s", dirFlagDef.DefValue)
	}

	allFlagDef := flags.Lookup("all")
	if allFlagDef == nil {
		t.Fatal("Expected 'all' flag to be registered")
	}
	if allFlagDef.DefValue != "false" {
		t.Errorf("Expected 'all' default to be false, got %s", allFlagDef.DefValue)
	}
}
