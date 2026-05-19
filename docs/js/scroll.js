/* ═══════════════════════════════════════════
   CICD — Scroll Reveal Animations
   ═══════════════════════════════════════════ */
(function() {
  // IntersectionObserver-based reveal (no GSAP dependency for lighter load)
  const reveals = document.querySelectorAll('.reveal');

  if (!reveals.length) return;

  const observer = new IntersectionObserver((entries) => {
    entries.forEach((entry, index) => {
      if (entry.isIntersecting) {
        // Stagger delay based on data attribute or sibling index
        const delay = entry.target.dataset.delay || 0;
        setTimeout(() => {
          entry.target.classList.add('active');
        }, delay * 1);
        observer.unobserve(entry.target);
      }
    });
  }, {
    threshold: 0.1,
    rootMargin: '0px 0px -50px 0px'
  });

  reveals.forEach((el, i) => {
    // Auto-stagger for grid children
    if (!el.dataset.delay) {
      const parent = el.parentElement;
      if (parent) {
        const siblings = Array.from(parent.children).filter(c => c.classList.contains('reveal'));
        const idx = siblings.indexOf(el);
        el.dataset.delay = idx * 100;
      }
    }
    observer.observe(el);
  });

  // Nav scroll effect
  const nav = document.querySelector('.nav');
  if (nav) {
    window.addEventListener('scroll', () => {
      if (window.scrollY > 60) {
        nav.classList.add('scrolled');
      } else {
        nav.classList.remove('scrolled');
      }
    });
  }

  // Counter animation for hero stats
  document.querySelectorAll('[data-count]').forEach(el => {
    const target = parseInt(el.dataset.count);
    const suffix = el.dataset.suffix || '';
    let current = 0;
    const step = Math.ceil(target / 40);
    const timer = setInterval(() => {
      current += step;
      if (current >= target) {
        current = target;
        clearInterval(timer);
      }
      el.textContent = current + suffix;
    }, 30);
  });
})();
