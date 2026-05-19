/* ═══════════════════════════════════════════
   CICD — Interactive Command Generator
   ═══════════════════════════════════════════ */
(function() {
  const form = document.getElementById('gen-form');
  const output = document.getElementById('gen-output-code');
  if (!form || !output) return;

  function generateSecret() {
    const arr = new Uint8Array(16);
    crypto.getRandomValues(arr);
    return Array.from(arr, b => b.toString(16).padStart(2, '0')).join('');
  }

  // Auto-fill secret on load
  const secretField = document.getElementById('gen-secret');
  if (secretField && !secretField.value) {
    secretField.value = generateSecret();
  }

  function buildCommand() {
    const repo = document.getElementById('gen-repo').value.trim();
    const name = document.getElementById('gen-name').value.trim();
    const branch = document.getElementById('gen-branch').value.trim() || 'main';
    const port = document.getElementById('gen-port').value.trim() || '9641';
    const timeout = document.getElementById('gen-timeout').value.trim();
    const secret = document.getElementById('gen-secret').value.trim();
    const gitUser = document.getElementById('gen-git-user').value.trim();
    const gitPass = document.getElementById('gen-git-pass').value.trim();
    const notifyUrl = document.getElementById('gen-notify-url').value.trim();
    const publicIp = document.getElementById('gen-public-ip').value.trim();
    
    const email = document.getElementById('gen-email').value.trim();
    const smtpHost = document.getElementById('gen-smtp-host').value.trim();
    const smtpPort = document.getElementById('gen-smtp-port').value.trim();
    const smtpUser = document.getElementById('gen-smtp-user').value.trim();
    const smtpPass = document.getElementById('gen-smtp-pass').value.trim();

    let step1 = `# Step 1: Download CICD binary (Universal Installer)
curl -fsSL https://cicd.rf.gd/install.sh | sh`;

    let cmd = `sudo ./cicd`;
    if (repo) cmd += ` \\\n  --repo-url ${repo}`;
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

    let step2 = `\n# Step 2: Run it\n${cmd}`;

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
