#!/bin/bash

# This script runs Node.js applications.

# Set script name for logging
SCRIPT_NAME=$(basename "$0")

# Function for logging messages
log() {
  local level="$1"
  local message="$2"
  echo -e "[$SCRIPT_NAME] $level: $message"
}

# Function for starting an application
start() {
  local app_name="$1"
  local app_path="$2"
  local env="$3"
  local log_file="$4"

  log "INFO" "Starting application: $app_name..."
  if [ "$env" == "development" ]; then
    npm run start:dev --prefix "$app_path" > "$log_file" 2>&1 &
  elif [ "$env" == "production" ]; then
    npm run start:prod --prefix "$app_path" > "$log_file" 2>&1 &
  else
    log "ERROR" "Invalid environment! Choose 'development' or 'production'."
    exit 1
  fi
}

# Function for stopping an application
stop() {
  local app_name="$1"
  local app_path="$2"
  local log_file="$3"

  log "INFO" "Stopping application: $app_name..."
  pkill -f "npm run start --prefix '$app_path'" > "$log_file" 2>&1
}

# Function for restarting an application
restart() {
  local app_name="$1"
  local app_path="$2"
  local env="$3"
  local log_file="$4"

  log "INFO" "Restarting application: $app_name..."
  stop "$app_name" "$app_path" "$log_file"
  start "$app_name" "$app_path" "$env" "$log_file"
}

# Function for checking application status
status() {
  local app_name="$1"
  local app_path="$2"
  local log_file="$3"

  log "INFO" "Checking status of application: $app_name..."
  if pgrep -f "npm run start --prefix '$app_path'" > /dev/null; then
    log "INFO" "Application is running."
  else
    log "INFO" "Application is not running."
  fi
}

# Example application configurations (modify for your needs)
# application_name="my-app"
# application_path="/path/to/my-app"
# environment="development"
# log_file="/path/to/app.log"

# Example usage
# start "my-app" "/path/to/my-app" "development" "/path/to/app.log"
# stop "my-app" "/path/to/my-app" "/path/to/app.log"
# restart "my-app" "/path/to/my-app" "production" "/path/to/app.log"
# status "my-app" "/path/to/my-app" "/path/to/app.log"

# Add or modify application configurations directly in the script (optional)

# Example of running tests
# npm run test --prefix "$app_path" > "$log_file" 2>&1