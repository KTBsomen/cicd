/* ═══════════════════════════════════════════
   CICD — Interactive Command Generator
   ═══════════════════════════════════════════ */
(function() {
  const form = document.getElementById('gen-form');
  const output = document.getElementById('gen-output-code');
  if (!form || !output) return;

  function generateSecret() {
    if (window.crypto && window.crypto.getRandomValues) {
      const arr = new Uint8Array(16);
      window.crypto.getRandomValues(arr);
      return Array.from(arr, b => b.toString(16).padStart(2, '0')).join('');
    } else {
      // Robust insecure context fallback
      let secret = "";
      for (let i = 0; i < 32; i++) {
        secret += Math.floor(Math.random() * 16).toString(16);
      }
      return secret;
    }
  }

  // Auto-fill secret on load
  const secretField = document.getElementById('val_vps_webhook_secret_key');
  if (secretField && !secretField.value) {
    secretField.value = generateSecret();
  }

  function buildCommand() {
    const repo = document.getElementById('val_vps_repo_source').value.trim();
    const name = document.getElementById('val_vps_proj_slug').value.trim();
    const branch = document.getElementById('val_vps_git_branch_name').value.trim() || 'main';
    const port = document.getElementById('val_vps_listen_port').value.trim() || '9641';
    const timeout = document.getElementById('val_vps_deploy_timeout_limit').value.trim();
    const secret = document.getElementById('val_vps_webhook_secret_key').value.trim();
    const gitUser = document.getElementById('val_git_auth_username').value.trim();
    const gitPass = document.getElementById('val_git_auth_token_secret').value.trim();
    const notifyUrl = document.getElementById('val_notify_webhook_channel').value.trim();
    const publicIp = document.getElementById('val_vps_public_ip_override').value.trim();
    
    const email = document.getElementById('val_admin_notification_email').value.trim();
    const smtpHost = document.getElementById('val_smtp_server_hostname').value.trim();
    const smtpPort = document.getElementById('val_smtp_server_port_num').value.trim();
    const smtpUser = document.getElementById('val_smtp_auth_user').value.trim();
    const smtpPass = document.getElementById('val_smtp_auth_pass_token').value.trim();

    let step1 = `# Step 1: Download CICD binary (Universal Installer)
curl -fsSL https://github.com/KTBsomen/cicd/releases/latest/download/install.sh | sh`;

    let step2Header = "";
    let cmd = `sudo ./cicd`;

    if (repo) {
      step2Header = `# Step 2: Run in Single-Project Auto-Provisioning Mode
# Pushing commits to this repository will automatically trigger deployments.`;
      cmd += ` \\\n  --repo-url ${repo}`;
      if (name) cmd += ` \\\n  --service-name ${name}`;
      if (branch && branch !== 'main') cmd += ` \\\n  --branch ${branch}`;
      if (port && port !== '9641') cmd += ` \\\n  --webhook ${port}`;
      if (timeout) cmd += ` \\\n  --deploy-timeout ${timeout}`;
      if (secret) cmd += ` \\\n  --webhook-secret ${secret}`;
      if (gitUser) cmd += ` \\\n  --git-username ${gitUser}`;
      if (gitPass) cmd += ` \\\n  --git-password ${gitPass}`;
      if (notifyUrl) cmd += ` \\\n  --notify-url ${notifyUrl}`;
      if (publicIp) cmd += ` \\\n  --public-ip ${publicIp}`;
      if (email) cmd += ` \\\n  --admin-email ${email}`;
      if (smtpHost) cmd += ` \\\n  --smtp-host ${smtpHost}`;
      if (smtpPort) cmd += ` \\\n  --smtp-port ${smtpPort}`;
      if (smtpUser) cmd += ` \\\n  --smtp-user ${smtpUser}`;
      if (smtpPass) cmd += ` \\\n  --smtp-pass ${smtpPass}`;
    } else {
      step2Header = `# Step 2: Run in Multi-Project Gateway & Dashboard Mode
# Boots the server-wide Command Center dashboard on port ${port}.
# Open the dashboard in your browser to visually register and manage projects.`;
      if (port && port !== '9641') cmd += ` \\\n  --webhook ${port}`;
      if (secret) cmd += ` \\\n  --webhook-secret ${secret}`;
      if (publicIp) cmd += ` \\\n  --public-ip ${publicIp}`;
      if (email) cmd += ` \\\n  --admin-email ${email}`;
      if (smtpHost) cmd += ` \\\n  --smtp-host ${smtpHost}`;
      if (smtpPort) cmd += ` \\\n  --smtp-port ${smtpPort}`;
      if (smtpUser) cmd += ` \\\n  --smtp-user ${smtpUser}`;
      if (smtpPass) cmd += ` \\\n  --smtp-pass ${smtpPass}`;
    }

    let step2 = `\n${step2Header}\n${cmd}`;

    output.textContent = step1 + '\n' + step2;
  }

  // Bind all inputs
  form.querySelectorAll('input').forEach(input => {
    input.addEventListener('input', buildCommand);
  });

  // Initial build
  buildCommand();

  // Copy button
  const copyBtn = document.getElementById('copy-cmd');
  if (copyBtn) {
    copyBtn.addEventListener('click', () => {
      navigator.clipboard.writeText(output.textContent).then(() => {
        const orig = copyBtn.textContent;
        copyBtn.textContent = 'Copied!';
        copyBtn.style.background = '#4ade80';
        setTimeout(() => {
          copyBtn.textContent = orig;
          copyBtn.style.background = '';
        }, 2000);
      });
    });
  }

  // Regenerate secret button
  const regenBtn = document.getElementById('regen-secret');
  if (regenBtn) {
    regenBtn.addEventListener('click', () => {
      secretField.value = generateSecret();
      buildCommand();
    });
  }
})();
