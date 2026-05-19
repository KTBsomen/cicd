/* ═══════════════════════════════════════════
   CICD — Docs Navigation & Utilities
   ═══════════════════════════════════════════ */
(function() {
  // ─── Sidebar active link tracking ───
  const sections = document.querySelectorAll('[data-section]');
  const sidebarLinks = document.querySelectorAll('.sidebar-link[href]');

  if (sections.length && sidebarLinks.length) {
    const observer = new IntersectionObserver((entries) => {
      entries.forEach(entry => {
        if (entry.isIntersecting) {
          const id = entry.target.getAttribute('data-section');
          sidebarLinks.forEach(link => {
            link.classList.toggle('active', link.getAttribute('href') === '#' + id);
          });
        }
      });
    }, { threshold: 0.2, rootMargin: '-80px 0px -60% 0px' });

    sections.forEach(s => observer.observe(s));
  }

  // ─── Mobile sidebar toggle ───
  const toggle = document.getElementById('sidebar-toggle');
  const sidebar = document.querySelector('.docs-sidebar');
  const overlay = document.getElementById('sidebar-overlay');

  if (toggle && sidebar) {
    toggle.addEventListener('click', () => {
      sidebar.classList.toggle('open');
      if (overlay) overlay.classList.toggle('active');
    });
    if (overlay) {
      overlay.addEventListener('click', () => {
        sidebar.classList.remove('open');
        overlay.classList.remove('active');
      });
    }
    // Close sidebar on link click (mobile)
    sidebarLinks.forEach(link => {
      link.addEventListener('click', () => {
        if (window.innerWidth <= 900) {
          sidebar.classList.remove('open');
          if (overlay) overlay.classList.remove('active');
        }
      });
    });
  }

  // ─── Code tabs ───
  document.querySelectorAll('.code-tabs').forEach(tabGroup => {
    const btns = tabGroup.querySelectorAll('.code-tab-btn');
    const contents = tabGroup.querySelectorAll('.code-tab-content');

    btns.forEach(btn => {
      btn.addEventListener('click', () => {
        const target = btn.dataset.tab;
        btns.forEach(b => b.classList.toggle('active', b === btn));
        contents.forEach(c => c.classList.toggle('active', c.dataset.tab === target));
      });
    });
  });

  // ─── FAQ accordion ───
  document.querySelectorAll('.faq-q').forEach(q => {
    q.addEventListener('click', () => {
      const item = q.closest('.faq-item');
      // Close others
      document.querySelectorAll('.faq-item.open').forEach(other => {
        if (other !== item) other.classList.remove('open');
      });
      item.classList.toggle('open');
    });
  });

  // ─── Dynamic Syntax Highlighting ───
  const hljsScript = document.createElement('script');
  hljsScript.src = 'https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js';
  hljsScript.onload = () => {
    hljs.configure({ ignoreUnescapedHTML: true });
    document.querySelectorAll('pre code').forEach((el) => {
      hljs.highlightElement(el);
    });
  };
  document.head.appendChild(hljsScript);
})();
