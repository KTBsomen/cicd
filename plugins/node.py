from . import EnvironmentSetup, register_plugin
import os
import subprocess
import pymongo
from pymongo import MongoClient

@register_plugin('node')
class NodeEnvironmentSetup(EnvironmentSetup):
    def install_dependencies(self):
        """Install Node.js and pm2 if not installed."""
        print("Checking for Node.js and pm2...")

        # Check for Node.js
        if subprocess.call(['which', 'node']) != 0:
            print("Node.js not found. Installing Node.js and npm...")
            os.system('echo sudo apt update && sudo apt install -y nodejs npm')
        else:
            print("Node.js already installed.")

        # Check for pm2
        if subprocess.call(['which', 'pm2']) != 0:
            print("pm2 not found. Installing pm2 globally...")
            os.system('sudo npm install -g pm2')
        else:
            print("pm2 already installed.")

    def clone_code(self):
        """Clone or pull the latest code from the repository."""
        print(f"Cloning or pulling code from {self.repo_url}...")

        if os.path.exists('codebase'):
            # If the repo exists, pull the latest changes
            print("Codebase already exists. Pulling the latest changes...")
            os.chdir('codebase')
            os.system('git reset --hard')
            os.system('git pull origin main')
        else:
            # Clone the repository for the first time
            print("Cloning the repository for the first time...")
            if self.git_credentials:
                protocol, repo = self.repo_url.split("://")
                repo_url = f"{protocol}://{self.git_credentials}@{repo}"
                os.system(f'git clone {repo_url} codebase')
            else:
                os.system(f'git clone {self.repo_url} codebase')
            os.chdir('codebase')

    def setup_server(self):
        """Install npm packages and start the Node.js server using pm2."""
        print("Setting up and starting the Node.js server with pm2...")

        # Install npm packages
        os.system('npm install')

        # Start the app with pm2 and set it to restart on changes
        os.system('pm2 delete myapp || true')  # Remove previous instance if exists
        os.system('pm2 start . --name myapp --env production --watch')

        # Save pm2 list for auto-restart on server reboot
        os.system('pm2 save')

    def start_monitoring(self):
        """Monitor the MongoDB for changes."""
        print("Starting MongoDB change monitoring...")

        # Connect to MongoDB
        client = MongoClient(self.mongodb_uri)
        db = client['cicd']  # Assume 'cicd' is the database name
        collection = db['latest_commits']  # Collection for storing the latest commits

        # Start listening to MongoDB change streams
        with collection.watch() as stream:
            for change in stream:
                print("Detected MongoDB change:", change)

                # Here you can add logic to pull the latest code and restart the server
                if change['operationType'] == 'update' and 'commit_hash' in change['updateDescription']['updatedFields']:
                    print("Code update detected. Pulling latest changes and restarting the server...")
                    os.chdir('../')  # Move out of codebase directory
                    self.clone_code()  # Pull the latest code
                    self.setup_server()  # Restart the server
