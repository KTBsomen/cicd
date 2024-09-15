document.addEventListener('DOMContentLoaded', () => {
    const loginForm = document.getElementById('login-form');
    const commandForm = document.getElementById('command-form');
    const message = document.getElementById('message');
    const output = document.getElementById('output');
    const password = document.getElementById('password');

   

    if (loginForm) {
        loginForm.addEventListener('submit', async (event) => {
            
            event.preventDefault();
            event.target.classList.add("loader")
            message.textContent=""
            const email = document.getElementById('email').value;
            
            const response = await fetch('/start_session', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email })
            });
            
            const data = await response.json();
            message.innerHTML =`${data.message} <br><a href="/dashboard">Click to visit Dashboard</a>`;
            event.target.classList.remove("loader")

        });
    }

    if (commandForm) {
        const socket = io('/commands');
        socket.on('connect', () => {
            console.log('Connected to server');
        });
        socket.on('disconnect', () => {
            console.log('Disconnected from server');
        });

        socket.on('output', (data) => {
            const outputElement = document.getElementById('output');
            outputElement.innerHTML += data.data + '<br>';
        });
        commandForm.addEventListener('submit', async (event) => {
            event.preventDefault();
            const command = document.getElementById('command').value;
            const password =document.getElementById('password').value.trim();
            if(!password){return}
            const response = await fetch('/verify_password', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password })
            });

            if (response.ok) {
                
                    let finalCommand = command;
                    let sudoPassword = '';
    
                    if (command.toLowerCase().includes('sudo')) {
                        sudoPassword = prompt('This command requires sudo. Please enter your sudo password:');
                        if (sudoPassword === null) return; // User cancelled the prompt
                        finalCommand = `echo ${sudoPassword.trim()} | sudo -S ${command}`;
                    }
                
                                const commandResponse = await fetch('/run_command', {
                                    method: 'POST',
                                    headers: { 'Content-Type': 'application/json' },
                                    body: JSON.stringify({ command: finalCommand, password })
                                });
                                
                
           

                const reader = commandResponse.body.getReader();
                const decoder = new TextDecoder();
                output.textContent = '';
                
                while (true) {
                    const { done, value } = await reader.read();
                    if (done) break;
                    output.textContent += decoder.decode(value);
                }
            } else {
                message.textContent = 'Invalid password.';
            }
        });
    }
});
