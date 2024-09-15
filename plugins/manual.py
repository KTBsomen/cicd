import os
import subprocess
import threading
import time
import pymongo
from pymongo import MongoClient
from plugins import EnvironmentSetup, register_plugin
import smtplib
from email.mime.text import MIMEText
from datetime import datetime
import asyncio


@register_plugin('manual')
class ManualEnvironmentSetup(EnvironmentSetup):
    def check_cicd_folder(self):
        """Ensure that the .cicd folder exists in the codebase."""
        cicd_path = os.path.join(os.getcwd(), '.cicd')
        if not os.path.exists(cicd_path):
            raise FileNotFoundError(f"'.cicd' folder not found in the codebase at {cicd_path}.")
        return cicd_path

    def send_error_email(self, step, error_log, smtp_host, smtp_port, smtp_user, smtp_pass, admin_email):
        """Send an email notification when an error occurs."""
        sender = smtp_user
        receivers = admin_email
        msg = MIMEText(f"Error during {step}: \n{error_log}")
        msg['Subject'] = f'Deployment Error at Step: {step}'
        msg['From'] = sender
        msg['To'] = receivers

        try:
            with smtplib.SMTP_SSL(smtp_host, smtp_port) as server:
                server.login(smtp_user, smtp_pass)
                server.sendmail(sender, [receivers], msg.as_string())
            print("Error email sent.")
        except Exception as e:
            print(f"Failed to send email: {e}")

    def run_shell_script(self, script_path, step_name, smtp_host, smtp_port, smtp_user, smtp_pass, admin_email):
        """Run a shell script and handle errors with email notification."""
        try:
            if os.path.exists(script_path):
                # Run the shell script with sudo password handling
                command = f"echo '{self.sudo_password}' | sudo -S bash {script_path}"
                
    # Use subprocess.Popen for non-blocking execution
                process = subprocess.Popen(command, shell=True, text=True, stdout=subprocess.PIPE,
                                        stderr=subprocess.PIPE, bufsize=1, universal_newlines=True)
                send_error_email=self.send_error_email
                # Non-blocking log
                def log_output(stream,typeofstream):
                    for line in iter(stream.readline, ''):
                        print(line, end='')
                        
                        with open('deployment.log', 'a') as log_file:
                            log_file.write(f"{typeofstream}=[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] [{command}] >>> {line.strip()}\n")
                        if typeofstream=="ERROR":
                            threading.Thread(target=send_error_email,args=(step_name, f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] [{command}] >>> {line.strip()}", smtp_host, smtp_port, smtp_user, smtp_pass, admin_email)).start()
    
                            
                        
                stdout_thread = threading.Thread(target=log_output, args=(process.stdout,"OUTPUT"))
                stderr_thread = threading.Thread(target=log_output, args=(process.stderr,"ERROR"))
                stdout_thread.start()
                stderr_thread.start()

                # Wait for the process to complete
                return_code=process.wait()

                # Wait for logging threads to finish
                stdout_thread.join()
                stderr_thread.join()
    

               
                if return_code != 0:
                    raise Exception(f"Process failed with return code {return_code}")

                # Capture stdout and stderr
    

    
                
                
            
            else:
                raise Exception(f"Script not found: {script_path}")
        
        except Exception as e:
            error_message = str(e)
            print(f"Error during {step_name}: {error_message}")
            self.send_error_email(step_name, error_message, smtp_host, smtp_port, smtp_user, smtp_pass, admin_email)
            
    def install_dependencies(self):
        if os.path.exists('codebase')==False:
            print("Codebase not found. pulling the latest code...")
            self.clone_code()
        else:
            os.chdir('codebase')
            """Run the install.sh script from the .cicd folder."""
            print("Running manual install from .cicd/install.sh ...")
            cicd_path = self.check_cicd_folder()
            self.run_shell_script(os.path.join(cicd_path, 'install.sh'), 'install', 
                                self.smtp_host, self.smtp_port, self.smtp_user, 
                                self.smtp_pass, self.admin_email)
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
        """Run the run.sh script from the cicd folder."""
        print("Running manual server startup from .cicd/run.sh ...")
        cicd_path = self.check_cicd_folder()
        self.run_shell_script(os.path.join(cicd_path, 'run.sh'), 'run', 
                              self.smtp_host, self.smtp_port, self.smtp_user, 
                              self.smtp_pass, self.admin_email)


    def start_monitoring(self):
        """Monitor MongoDB for changes and rerun the server on code changes."""
        print("Starting MongoDB change monitoring...")
        try:
            # Connect to MongoClient and monitor the changes
            client = MongoClient(self.mongodb_uri)
            db = client['cicd']  # Assume 'cicd' is the database name
            collection = db['latest_commits']  # Collection to monitor the latest commits

            # Monitor MongoDB change streams for specific repo_url
            # pipeline = [
            #     {'$match': {'fullDocument.repourl': self.repo_url}}
            # ]
            with collection.watch(full_document='updateLookup') as stream:
                for change in stream:
                    
                    print("Detected MongoDB change:", change)

                    if 'fullDocument' not in change:
                        continue
                    if 'repourl' not in change['fullDocument'] or change['fullDocument']['repourl'] != self.repo_url:
                        continue
                    with open('deployment.log', 'a') as log_file:
                        log_file.write(f"MongoDB change detected: {change}\n")
                    # Detecting updates in commit information
                    if change['operationType'] == 'insert' :
                        print("Code insert detected. Pulling latest changes and restarting the server...")

                        # Pull latest code changes
                        os.chdir('../')  # Go back to the base directory
                        self.clone_code()  # Pull latest code and restart the server
                        try:
                            threading.Thread(target=self.setup_server).start()
                        except Exception as e:
                            print(f"Error starting server: {e}")
                    elif (change['operationType'] == 'update' and 'commit_hash' in change['updateDescription']['updatedFields'] and not change['updateDescription']['updatedFields']['commit_hash'].startswith("newRun")):
                        print("Code update detected. Pulling latest changes and restarting the server...")

                        # Pull latest code changes
                        os.chdir('../')  # Go back to the base directory
                        self.clone_code()  # Pull latest code and restart the server
                        try:
                            threading.Thread(target=self.setup_server).start()
                        except Exception as e:
                            print(f"Error starting server: {e}")
                    else:
                        print("No changes detected.")


        except:
            print("Error starting MongoDB change monitoring.")