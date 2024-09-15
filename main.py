
import argparse
import importlib
import json
import os
import random
import shlex
import smtplib
import hmac
import hashlib
from email.mime.text import MIMEText
import socket
import string
import sys
import threading
import time
from urllib import parse, request

from pymongo import MongoClient
from plugins import plugins

from http.server import BaseHTTPRequestHandler, HTTPServer

from datetime import datetime
import subprocess
import shutil
import platform
from pathlib import Path
import getpass
def install_main_packages(package_name, install_command, sudo_password=None, username=None):
    """Install a package if not already installed and import it."""
    # Check if the package is installed
    if shutil.which(package_name):
        print(f"{package_name} is already installed.")
    else:
        print(f"{package_name} is not installed. Installing...")
        # Prepare the command for installation
        if sudo_password:
            if username:
                install_command = ['sudo', '-S', '-u', username] + install_command
            else:
                install_command = ['sudo', '-S'] + install_command
            result = subprocess.run(install_command, input=sudo_password + '\n', text=True, capture_output=True)
        else:
            if username:
                install_command = ['sudo', '-u', username] + install_command
            result = subprocess.run(install_command, text=True, capture_output=True)
        
        if result.returncode != 0:
            print(f"Failed to install {package_name}: {result.stderr}")
            
            
        else:
            print(f"{package_name} installed successfully.")

    # Import the package
    try:
        importlib.import_module(package_name,"MongoClient")
        print(f"{package_name} imported successfully.")
    except ImportError:
        print(f"Failed to import {package_name}.")
        

def get_package_manager():
    """Return the appropriate package manager based on the operating system."""
    os_name = platform.system().lower()
    if os_name == 'linux':
        if shutil.which('apt'):
            return 'apt'
        elif shutil.which('yum'):
            return 'yum'
        elif shutil.which('dnf'):
            return 'dnf'
    elif os_name == 'darwin':  # macOS
        if shutil.which('brew'):
            return 'brew'
    elif os_name == 'windows':
        return None  # Windows does not have a native package manager in the same sense
    return None

def detect_platform():
    system = platform.system().lower()
    if 'linux' in system:
        return 'linux'
    elif 'darwin' in system:
        return 'macos'
    elif 'windows' in system:
        return 'windows'
    else:
        raise Exception(f"Unsupported platform: {system}")

def run_sudo_command(command, sudo_password):
    if sudo_password:
        full_command = f"echo {sudo_password} | sudo -S {command}"
        result = subprocess.run(full_command, shell=True, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    else:
        result = subprocess.run(f"sudo {command}", shell=True, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    
    print(f"Command output:\n{result.stdout}")
    if result.stderr:
        print(f"Command error:\n{result.stderr}")
    
    return result
def create_systemd_service(service_name, exec_path,Workingdirectory="/home/",user="root" ,description="Auto-generated service", sudo_password=None):
    systemd_service = f"""
[Unit]
Description={description}
After=network.target

[Service]
ExecStartPre=git config --global --add safe.directory { Workingdirectory[:-1] if Workingdirectory[-1]=='/' else Workingdirectory}/codebase
ExecStart={exec_path}
WorkingDirectory={Workingdirectory}
User={user}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
"""
    service_path = f"/etc/systemd/system/{service_name}.service"
    temp_path = f"/tmp/{service_name}.service"

    try:
        # Write the systemd service file to a temporary location
        with open(temp_path, 'w') as service_file:
            service_file.write(systemd_service)

        # Function to run commands with sudo

        # Move the service file to the correct location
        run_sudo_command(f"mv {temp_path} {service_path}",sudo_password)

        # Set correct permissions
        run_sudo_command(f"chmod 644 {service_path}",sudo_password)

        # Reload systemd to pick up the new service
        run_sudo_command("systemctl daemon-reload",sudo_password)

        # Enable and start the service
        run_sudo_command(f"systemctl enable {service_name}",sudo_password)
        run_sudo_command(f"systemctl start {service_name}",sudo_password)

        print(f"{service_name} service created and started successfully!")

    except subprocess.CalledProcessError as e:
        print(f"Failed to create or start systemd service: {e}")
        print(f"Command output: {e.output.decode()}")
        print(f"Command error: {e.stderr.decode()}")
    except Exception as e:
        print(f"An unexpected error occurred: {str(e)}")
def send_error_email(error_log,subject, smtp_host, smtp_port, smtp_user, smtp_pass, admin_email):
    sender = smtp_user
    receivers = admin_email
    msg = MIMEText(error_log)
    msg['Subject'] = subject
    msg['From'] = sender
    msg['To'] = receivers

    try:
        with smtplib.SMTP_SSL(smtp_host, smtp_port) as server:
            server.login(smtp_user, smtp_pass)
            server.sendmail(sender, [receivers], msg.as_string())
        print("Error email sent.")
    except Exception as e:
        print(f"Failed to send email: {e}")

def verify_signature(secret, signature, data):
    """Verify GitHub Webhook signature."""
    hash_name, signature = signature.split('=')
    hmac_digest = hmac.new(secret.encode(), data, hashlib.sha256).hexdigest()
    return hmac.compare_digest(hmac_digest, signature)


def generate_password(length=12):
    """Generate a random password."""
    characters = string.ascii_letters + string.digits + string.punctuation
    return ''.join(random.choice(characters) for _ in range(length))

def get_public_ip():
    """Get the public IP address of the server."""
    try:
        # Using ipify API to get the public IP
        response = request.urlopen('https://api.ipify.org')
        if response.status == 200:
            ip = response.read().decode('utf-8').strip()
            # if not ip.startswith(('10.', '172.', '192.168.', '127.')):
            return ip
        else:
            raise Exception(f"Failed to get IP. Status code: {response.status}")
    except Exception as e:
        print(f"Error occurred while fetching public IP from ipify: {e}")

    try:
        # Fallback to socket if ipify fails
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
        # if not ip.startswith(('10.', '172.', '192.168.', '127.')):
        return ip
    except Exception as e:
        print(f"Error occurred while fetching IP using socket: {e}")

    print("Failed to get a public IP address")
    with open("deployment.log","a") as f:
        f.write(f"ERROR = [{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] >>> Failed to get a public IP address\n")
    return None


sessions = {}  # Store session data with email as key and password as value
password_expiration =  5*60  # Password expiration time in seconds (5 minutes)


class WebhookHandler(BaseHTTPRequestHandler):
    """Handle incoming webhook requests."""
    
    def __init__(self, secret, *args, **kwargs):
        self.secret = secret
        super().__init__(*args, **kwargs)
    def do_GET(self):
        """Handle GET requests with routing logic."""
        parsed_path = parse.urlparse(self.path)
        route = parsed_path.path

        if route == '/':
            self.handle_main_page()
        elif route == '/verify':
            self.handle_verification()
        elif route == '/login':
            self.handle_data_access(parsed_path.query)
        else:
            self.send_response(404)
            self.end_headers()
            self.wfile.write(b'Not Found')
    def handle_verification(self):
        """Handle the verification request."""
        self.send_response(200)
        self.send_header('Content-type', 'text/plain')
        self.end_headers()
        self.wfile.write(b'Verification successful')
    
    def handle_data_access(self, query_params):
        """Handle data access requests."""
        query_params = parse.parse_qs(query_params)
        password = query_params.get('password', [''])[0]
        session_password, timeout = sessions.get(args.admin_email, (None, None))
        if not  password:
            self.send_response(400)
            self.end_headers()
            self.wfile.write(b'Missing password')
            return
        if session_password != password:
            self.send_response(401)
            self.end_headers()
            self.wfile.write(b'Unauthorized')
            return
        if time.time() - timeout > password_expiration:
            self.send_response(401)
            self.end_headers()
            self.wfile.write(b'Password expired')
            return
        sessions[args.admin_email] = time.time()
        self.send_response(200)
        self.send_header('Content-type', 'application/json')
        self.end_headers()
        client = MongoClient(args.mongodb_uri)
        db = client['cicd']  # Database name from the default URI
        collection = db['latest_commits']
        data = list(collection.find({'repourl': args.repo_url}, {'_id': 0}))
        self.wfile.write(json.dumps(data, default=str).encode('utf-8'))
        
    def handle_main_page(self):
        password = generate_password()
        try:
            session_password, timeout = sessions.get(args.admin_email, (None, 0))
        except:
            session_password, timeout = sessions.get(args.admin_email, (None, 0)) if isinstance(sessions.get(args.admin_email, (None, 0)), tuple) else (None, 0)


        if not session_password or time.time() - timeout > password_expiration:
            sessions[args.admin_email] = (password, time.time())
            send_error_email(f"Login with this password:  {password} ",
                    'Web UI Password',
                    args.smtp_host, 
                    args.smtp_port, 
                    args.smtp_user, 
                    args.smtp_pass, 
                    args.admin_email                        
                    )
        self.send_response(200)
        self.send_header('Content-type', 'text/html')
        self.end_headers()
        with open( os.path.join(os.path.dirname(__file__), 'index.html'), 'rb') as file:
            self.wfile.write(file.read())
    def do_POST(self):
        content_length = int(self.headers['Content-Length'])
        post_data = self.rfile.read(content_length)

        # Verify the webhook signature
        signature = self.headers.get('X-Hub-Signature-256')
        if not signature:
            self.send_response(400)
            self.end_headers()
            self.wfile.write(b'Missing signature')
            return
        
        if not verify_signature(self.secret, signature, post_data):
            self.send_response(403)
            self.end_headers()
            self.wfile.write(b'Signature verification failed')
            return

        # Process the webhook payload
        try:
            payload = json.loads(post_data)
            commit_hash = payload['head_commit']['id']
            branch = payload['ref'].split('/')[-1]
            author = payload['head_commit']['author']['name']

            # Update MongoDB
            update_mongodb(commit_hash, branch, author, self.server.mongodb_uri,self.server.repo_url,self.server.public_ip)

            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'Webhook received and processed')

        except Exception as e:
            self.send_response(500)
            self.end_headers()
            self.wfile.write(f"Error processing webhook: {str(e)}".encode())

def update_mongodb(commit_hash, branch, author, mongodb_uri,repo_url ,public_ip=None):
    """Update MongoDB collection with the latest commit info."""
    client = MongoClient(mongodb_uri)
    db = client['cicd']  # Database name from the default URI
    collection = db['latest_commits']  # Change this to your collection name

    # Update the latest code version
  # Define the update operations
    update_operations = {
        '$set': {
            'commit_hash': commit_hash,
            'branch': branch,
            'updated_at': datetime.now(),
            'author': author,
        }
    }

    # If a new IP is provided, add it to the 'publicIps' list if it doesn't exist
    if public_ip:
        update_operations['$addToSet'] = {'publicIps': public_ip}

    # Perform the update operation
    result = collection.update_one(
        {"repourl": repo_url},
        update_operations,
        upsert=True
    )
    print("\n========update============\n",result)

def run_webhook_server(port, secret, mongodb_uri,repourl,counter=0,public_ip=None):
    if counter > 5:
        print("Failed to start webhook server. Exiting...")
        return
    """Start a webhook listener server."""
    try:
        server_address = ('', port)
        httpd = HTTPServer(server_address, lambda *args: WebhookHandler(secret, *args))
        httpd.mongodb_uri = mongodb_uri  # Set MongoDB URI for the server
        httpd.repo_url=repourl
        httpd.public_ip=public_ip
        print(f"Webhook listener running on port {port} >>> Add this URL to GitHub Webhook settings. http://{get_public_ip()}:{port}. ")
        
        httpd.serve_forever()
    except:
        print("OSError: Webhook listener already listening. killing it...")
        subprocess.call(f"lsof -t -i:{port} | xargs -r kill -9 ",shell=True)
        run_webhook_server(port, secret, mongodb_uri,repourl,counter=counter+1,public_ip=public_ip)
        

def main():
    # Parse command-line arguments
    parser = argparse.ArgumentParser(description="Setup production environment, monitor changes, and optionally enable webhook listener.")
    parser.add_argument('--setup', type=str, help='Type of setup (e.g., node, python,manual)', required=True)
    parser.add_argument('--repo-url', type=str, help='Repository URL for the code', required=True)
    
    # MongoDB URI argument with default value
    parser.add_argument('--mongodb-uri', type=str, help='MongoDB URI for change monitoring', 
                        default='mongodb+srv://boutiquelp24:tOVIkcoRqLsw03p9@boutiquemain.7rglrir.mongodb.net/cicd?retryWrites=true&w=majority&appName=boutiqueMain')
    
    # Git credentials for private repositories
    parser.add_argument('--git-username', type=str, help='Git username for private repos', required=False)
    parser.add_argument('--git-password', type=str, help='Git password/token for private repos', required=False)
    
    # SMTP arguments with default values
    parser.add_argument('--smtp-host', type=str, help='SMTP host for sending error emails', 
                        default='smtpout.secureserver.net')
    parser.add_argument('--smtp-port', type=int, help='SMTP port for sending error emails', 
                        default=465)
    parser.add_argument('--smtp-user', type=str, help='SMTP username for sending error emails', 
                        default='hello@wowcircle.in')
    parser.add_argument('--smtp-pass', type=str, help='SMTP password for sending error emails', 
                        default='WOWCIRCLE@123#')
    parser.add_argument('--admin-email', type=str, help='Admin email to send error logs', required=True)
    parser.add_argument('--user', type=str, help='username of the code runner', required=False)
    parser.add_argument('--sudo-pass', type=str, help='sudo password, we need this as we have to install packages', required=False)

# Service creation arguments with default values
    parser.add_argument('--service-name', type=str, help='Name of the service to create', default='myapp')
    parser.add_argument('--service-dir', type=str, help='path of the service to create files and folders',default='/home/')
    parser.add_argument('--service-user', type=str, help='path of the service to create files and folders',default='root')
    parser.add_argument('--service-reset', type=str, help='path of the service to create files and folders',required=False)
    


    
    # Webhook listener arguments
    parser.add_argument('--webhook', type=int, help='Port number to run the webhook listener', required=False)
    parser.add_argument('--webhook-secret', type=str, help='GitHub Webhook secret', required=False)
    
    # public ip 
    parser.add_argument('--public-ip', type=str, help='Public Ip for managing it via web UI on port 9641',default=get_public_ip())
    
    global args
    args = parser.parse_args()
    print(args)
   
    # Get Git credentials if private repository
    git_credentials = None
    if args.git_username and args.git_password:
        git_credentials = f"{args.git_username}:{args.git_password}"

    # If webhook flag is provided, start the webhook listener
    if args.webhook and args.webhook_secret:
        webhook_thread = threading.Thread(target=run_webhook_server, args=(args.webhook, args.webhook_secret, args.mongodb_uri,args.repo_url,0,args.public_ip))
        webhook_thread.start()

    # Select and run the appropriate plugin
    if args.setup in plugins:
        if args.sudo_pass:
            environment_setup = plugins[args.setup](args.repo_url, git_credentials, args.mongodb_uri, sudopass=args.sudo_pass, user=args.user,smtp_host=args.smtp_host, smtp_port=args.smtp_port, smtp_user=args.smtp_user, smtp_pass=args.smtp_pass,admin_email=args.admin_email)
        else:
            environment_setup = plugins[args.setup](args.repo_url, git_credentials, args.mongodb_uri,smtp_host=args.smtp_host, smtp_port=args.smtp_port, smtp_user=args.smtp_user, smtp_pass=args.smtp_pass,admin_email=args.admin_email)

        try:
  
            service_name = args.service_name  
            service_file_path = f"/etc/systemd/system/{service_name}.service"
            
            
            args_string = ' '.join([f'--{key.replace("_","-")} {shlex.quote(str(value))}' for key, value in vars(args).items() if value is not None and key !="service_reset"])
            
            
            execPathCommand=f"{sys.executable} {os.path.abspath(__file__)}  {args_string}"
            print(f"Arguments string: {execPathCommand}")
            subprocess.Popen(f"{sys.executable} {os.path.abspath(__file__).replace('main.py','customCommandExecutor.py')} {args_string}",shell=True,text=True,)
            if args.service_reset:
                print("Deleting and reloading systemd service")
                if os.path.exists(service_file_path):
                    # Stop the service
                    run_sudo_command('sudo systemctl stop '+ service_name,sudo_password=args.sudo_pass)
                    # Disable the service
                    run_sudo_command('sudo systemctl disable '+ service_name,sudo_password=args.sudo_pass)
                    # Remove the service file
                    run_sudo_command(f"sudo rm -r {service_file_path}",sudo_password=args.sudo_pass)
                    # Reload systemd
                    run_sudo_command('sudo systemctl daemon-reload ',sudo_password=args.sudo_pass)
                    print(f"Systemd service {service_name} has been deleted and reloaded")
                    subprocess.Popen(f"lsof -t -i:9641 | xargs -r kill -9 && {sys.executable} {os.path.abspath(__file__).replace('main.py','customCommandExecutor.py')} {args_string}",shell=True,text=True,)
                    
                else:
                    print(f"Service file {service_file_path} does not exist. Nothing to delete.")

            # Check if the systemd service file already exists
            if not os.path.exists(service_file_path):
                print("creating systemd registry")
                create_systemd_service(service_name=service_name,exec_path=execPathCommand,sudo_password=args.sudo_pass,user=args.service_user,Workingdirectory=args.service_dir)
                update_mongodb("newRun"+str(random.randint(10,5895333)), "main", "author", args.mongodb_uri,args.repo_url ,public_ip=get_public_ip())

            else:
                print(f"Systemd service file already exists at {service_file_path} running from there...")
                environment_setup.install_dependencies()

                if args.setup=="manual":
                    
                    setup_thread = threading.Thread(target=environment_setup.setup_server)
                    setup_thread.start()
                    environment_setup.start_monitoring()


                else:
                    environment_setup.clone_code()
                    setup_thread = threading.Thread(target=environment_setup.setup_server)
                    setup_thread.start()
                    monitoring_thread = threading.Thread(target=environment_setup.start_monitoring,daemon=True)
                    monitoring_thread.start()
                    print(f"Monitoring thread is alive: {monitoring_thread.is_alive()}")

                
            

            
        except Exception as e:
            error_log = f"Error: {str(e)}\nIP:"
            server_ip = socket.gethostbyname(socket.gethostname())
            send_error_email(
                error_log+ server_ip,
                'Deployment Error',
                args.smtp_host, 
                args.smtp_port, 
                args.smtp_user, 
                args.smtp_pass, 
                args.admin_email
            )
            print(f"Error: {e}")

    else:
        print(f"Unsupported setup type: {args.setup}")

if __name__ == "__main__":
    main()
