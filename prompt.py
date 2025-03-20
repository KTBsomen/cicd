import os
import re
import zipfile
import json
from google.generativeai import GenerativeModel
import google.generativeai as genai

genai.configure(api_key="AIzaSyD7HAIUV9KkB-oYc2HRoD6-5E02Dk5J314")

def generate_and_process_files(langorFram):
    # Initialize Gemini model
    model = GenerativeModel(model_name="gemini-1.5-flash")

    # Your prompt here
    prompt = f"""# Prompt for Generating Universal Development Environment Setup and Application Runner Scripts for Production use.

Create two Bash scripts: `install.sh` and `run.sh`, for setting up a development environment and running applications for {langorFram}, respectively for production grade applications. Replace {langorFram} with the specific programming language, framework, or technology requested (e.g., Python, Go, Rust, Flutter, Ruby, Java, etc.).

## General Requirements for Both Scripts:
1. Include detailed comments explaining each step.
2. Implement comprehensive error handling and logging.
3. Use best practices for Bash scripting.
4. Make the scripts user-friendly and informative.
5. Include comments on how to completely remove the installed components and revert changes.

## install.sh Requirements:
1. Check for and install necessary system dependencies (e.g., curl, wget, git).
2. Detect the operating system and adjust installation methods accordingly (support at least Ubuntu/Debian, CentOS/RHEL, macOS).
3. Check for existing installation of {langorFram}.
4. If {langorFram} is not installed or outdated:
   a. Install the latest stable version of {langorFram}.
   b. Use the official or most recommended installation method.
5. Install essential tools and package managers associated with {langorFram}.
6. Set up the development environment (e.g., virtual environments, SDKs).
7. Install commonly used libraries or frameworks for {langorFram}.
8. Add necessary binary paths to the system PATH if not already present.
9. Set up any required environment variables.
10. Verify all installations by checking versions and functionality.
11. Provide clear output at each step, including success messages or error notifications.
12. Include a section commenting out optional installations (e.g., IDEs, additional tools) that users can uncomment if needed.

## run.sh Requirements:
1. Check if necessary runtime components are installed.
2. Define functions to start, stop, and restart applications.
3. Include example configurations for running {langorFram} applications.
4. Allow users to easily add or modify application configurations directly in the script.
5. Use syntax and conventions familiar to {langorFram} developers.
6. Implement basic process management (start, stop, restart, status check).
7. Include options for running applications in development and production modes.
8. Provide logging options and log file management.
9. Implement basic error handling and reporting for application runtime issues.
10. Display clear instructions for any manual steps required.
11. Include examples of how to run tests, if applicable to {langorFram}.

## Specific Considerations for {langorFram}:
- Include any specific installation requirements or configurations needed for {langorFram}.
- Adjust the run.sh script to accommodate typical {langorFram} application structures and startup commands.
- If {langorFram} has any particular best practices for development or production deployment, incorporate them into the scripts.
- Include any language-specific or framework-specific tools that are commonly used (e.g., linters, formatters, build tools).

## Additional Requirements:
1. In install.sh, include commented-out instructions on how to completely remove {langorFram} and all installed components, reverting the system to its previous state.
2. Provide options in run.sh for different environment configurations (e.g., development, staging, production).
3. Include basic security best practices relevant to {langorFram}.
4. Add a section in install.sh for optional installation of popular development tools or IDEs relevant to {langorFram}.
5. In run.sh, include examples of how to run the application with different configurations or environment variables.

Ensure both scripts are comprehensive, secure, and follow best practices for {langorFram} development and deployment. The scripts should be easy to understand and modify for users with varying levels of experience, from beginners to advanced developers.
    
    
    """
    print("Please wait....\ngenerating files....")
    # Generate content
    response = model.generate_content(prompt)

    # Parse the response
    content = response.text
    files = parse_response(content)
    if not files:
        print("No file creation indication found in the response.")
        return None

    # Create .cicd folder
    os.makedirs('.cicd', exist_ok=True)
    # Create files
    for filename, file_content in files.items():
        with open(os.path.join('.cicd', filename), 'w') as f:
            f.write(file_content)

    # Zip the folder
    zip_path = 'cicd_files.zip'
    with zipfile.ZipFile(zip_path, 'w') as zipf:
        for root, _, files in os.walk('.cicd'):
            for file in files:
                zipf.write(os.path.join(root, file), 
                           os.path.relpath(os.path.join(root, file), 
                                           os.path.join('.cicd', '..')))
    os.remove('.cicd')
    return zip_path
files = {}

def parse_response(content):
    files = {}

    # Pattern to match file sections
    pattern = r"##\s+(.*?)\n```(.*?)```"
    matches = re.findall(pattern, content, re.DOTALL | re.MULTILINE)
    # Process matches
    for filename, file_content in matches:
        files[filename.strip()] = file_content.strip()

    return files

# Run the function
zip_file_path = generate_and_process_files("nodejs")
if zip_file_path:
    print(f"Files created and zipped. Zip file path: {zip_file_path}")
else:
    print("Failed to generate and process files.")