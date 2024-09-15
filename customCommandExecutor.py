import argparse
from email.mime.text import MIMEText
import hashlib
import hmac
import smtplib
from flask import Flask, request, jsonify, render_template, redirect, url_for
import subprocess
import random
import string
import os
import json
import time
from threading import Timer
from flask_socketio import SocketIO, emit

app = Flask(__name__)
socketio = SocketIO(app)
app.config['SECRET_KEY'] = 'secret!'
# Email configuration

# Parse command-line arguments
parser = argparse.ArgumentParser(description="Setup production environment, monitor changes, and optionally enable webhook listener.")



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
parser.add_argument('--public-ip', type=str, help='Public Ip for managing it via web UI on port 9641', required=False)



args = parser.parse_args()

print(f"\n=================\n{args.smtp_host,args.smtp_port,args.smtp_user}")
sessions = {}  # Store session data with email as key and password as value
password_expiration = 5 * 60  # Password expiration time in seconds (5 minutes)
def verify_signature(secret, signature, data):
    """Verify GitHub Webhook signature."""
    hash_name, signature = signature.split('=')
    hmac_digest = hmac.new(secret.encode(), data, hashlib.sha256).hexdigest()
    return hmac.compare_digest(hmac_digest, signature)

def generate_password(length=12):
    """Generate a random password."""
    characters = string.ascii_letters + string.digits + string.punctuation
    return ''.join(random.choice(characters) for _ in range(length))

def send_email(recipient_email, password):
    msg = MIMEText(f'Your session password is: {password}')
    msg['Subject'] = 'Your CICD Tool Session Password'
    msg['From'] = args.smtp_user
    msg['To'] = recipient_email

    with smtplib.SMTP_SSL(args.smtp_host, args.smtp_port) as server:
        server.login(args.smtp_user, args.smtp_pass)
        server.sendmail(args.smtp_user, recipient_email, msg.as_string())
        


def cleanup_sessions():
    """Remove expired sessions."""
    current_time = time.time()
    expired_emails = [email for email, (password, timestamp) in sessions.items() if current_time - timestamp > password_expiration]
    for email in expired_emails:
        del sessions[email]

@app.route('/')
def index():
    return render_template('index.html')

@app.route('/start_session', methods=['POST'])
def start_session():
    """Start a new session and send a password to the user."""
    data = request.json
    email = args.admin_email

    if not email:
        return jsonify({'message': 'Email is required'}), 400

    password = generate_password()
    sessions[email] = (password, time.time())
    send_email(email, password)

    response = jsonify({'message': 'Session started. Check your email for the password.'})
    response.set_cookie('email', email)
    return response
@app.route('/verify_password', methods=['POST'])
def verify_password():
    """Verify the session password."""
    data = request.json
    password = data.get('password')
    email = request.cookies.get('email')

    if not email or not password:
        return jsonify({'message': 'Password or email is missing'}), 400

    session_password, _ = sessions.get(email, (None, None))
    if session_password != password:
        return jsonify({'message': 'Invalid password'}), 403

    return jsonify({'message': 'Password verified'})


@app.route('/run_command', methods=['POST'])
def run_command():
    """Start a command and stream output via SocketIO."""
    data = request.json
    command = data.get('command')
    email = request.cookies.get('email')
    password = data.get('password')

    if not email or not password or not command:
        return jsonify({'message': 'Command, email, or password is missing'}), 400

    session_password, _ = sessions.get(email, (None, None))
    if session_password != password:
        return jsonify({'message': 'Invalid password'}), 403

    try:
        # Start command execution in a separate thread
        def execute_command():
            process = subprocess.Popen(command, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            for line in iter(process.stdout.readline, ''):
                socketio.emit('output', {'data': line}, namespace='/commands')
            for line in iter(process.stderr.readline, ''):
                socketio.emit('output', {'data': line}, namespace='/commands')
            process.wait()
            socketio.emit('output', {'data': 'Command execution completed.'}, namespace='/commands')

        socketio.start_background_task(execute_command)
        return jsonify({'message': 'Command execution started <br>========================'}), 200

    except Exception as e:
        return jsonify({'message': f"Error executing command: {str(e)}"}), 500

@socketio.on('connect', namespace='/commands')
def handle_connect():
    """Handle new SocketIO connections."""
    print('Client connected')
    emit('message', {'data': 'You are connected to the command execution server.'}, namespace='/commands')

@socketio.on('disconnect', namespace='/commands')
def handle_disconnect():
    """Handle SocketIO disconnections."""
    print('Client disconnected')

@app.route('/dashboard')
def dashboard():
    """Render the dashboard page."""
    return render_template('dashboard.html')

if __name__ == '__main__':
    # Run cleanup sessions periodically
    Timer(60, cleanup_sessions).start()
    socketio.run(app=app,debug=True, port=9641,use_reloader=False, allow_unsafe_werkzeug=True)
