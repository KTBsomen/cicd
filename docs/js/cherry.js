/* ═══════════════════════════════════════════
   CICD — Three.js Cherry Blossom Particles
   ═══════════════════════════════════════════ */
(function() {
  const canvas = document.getElementById('cherry-canvas');
  if (!canvas) return;

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(75, window.innerWidth / window.innerHeight, 0.1, 1000);
  camera.position.z = 5;

  const renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true });
  renderer.setSize(window.innerWidth, window.innerHeight);
  renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));

  // Petal geometry — a small flat ellipse
  const petalCount = 120;
  const positions = new Float32Array(petalCount * 3);
  const velocities = [];
  const rotations = [];
  const sizes = new Float32Array(petalCount);

  for (let i = 0; i < petalCount; i++) {
    positions[i * 3]     = (Math.random() - 0.5) * 20;     // x
    positions[i * 3 + 1] = (Math.random() - 0.5) * 20;     // y
    positions[i * 3 + 2] = (Math.random() - 0.5) * 10 - 2; // z

    velocities.push({
      x: (Math.random() - 0.5) * 0.003,
      y: -0.005 - Math.random() * 0.008,
      z: (Math.random() - 0.5) * 0.002,
      wobbleSpeed: 0.5 + Math.random() * 1.5,
      wobbleAmp: 0.01 + Math.random() * 0.02
    });

    rotations.push(Math.random() * Math.PI * 2);
    sizes[i] = 0.04 + Math.random() * 0.06;
  }

  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
  geometry.setAttribute('size', new THREE.BufferAttribute(sizes, 1));

  // Petal texture — draw a soft circle
  const texCanvas = document.createElement('canvas');
  texCanvas.width = 64;
  texCanvas.height = 64;
  const ctx = texCanvas.getContext('2d');
  const gradient = ctx.createRadialGradient(32, 32, 0, 32, 32, 32);
  gradient.addColorStop(0, 'rgba(255, 183, 197, 0.9)');
  gradient.addColorStop(0.4, 'rgba(255, 183, 197, 0.6)');
  gradient.addColorStop(0.7, 'rgba(255, 214, 224, 0.3)');
  gradient.addColorStop(1, 'rgba(255, 183, 197, 0)');
  ctx.fillStyle = gradient;
  ctx.beginPath();
  ctx.ellipse(32, 32, 30, 22, 0, 0, Math.PI * 2);
  ctx.fill();

  const texture = new THREE.CanvasTexture(texCanvas);

  const material = new THREE.PointsMaterial({
    map: texture,
    size: 0.12,
    sizeAttenuation: true,
    transparent: true,
    opacity: 0.7,
    depthWrite: false,
    blending: THREE.AdditiveBlending
  });

  const points = new THREE.Points(geometry, material);
  scene.add(points);

  // Subtle ambient glow
  const glowGeo = new THREE.SphereGeometry(0.5, 16, 16);
  const glowMat = new THREE.MeshBasicMaterial({
    color: 0xffb7c5,
    transparent: true,
    opacity: 0.03
  });
  const glow = new THREE.Mesh(glowGeo, glowMat);
  glow.scale.set(8, 8, 8);
  scene.add(glow);

  let time = 0;
  function animate() {
    requestAnimationFrame(animate);
    time += 0.016;

    const pos = geometry.attributes.position.array;
    for (let i = 0; i < petalCount; i++) {
      const v = velocities[i];
      pos[i * 3]     += v.x + Math.sin(time * v.wobbleSpeed) * v.wobbleAmp;
      pos[i * 3 + 1] += v.y;
      pos[i * 3 + 2] += v.z;

      // Reset petal if it falls below
      if (pos[i * 3 + 1] < -10) {
        pos[i * 3]     = (Math.random() - 0.5) * 20;
        pos[i * 3 + 1] = 10 + Math.random() * 3;
        pos[i * 3 + 2] = (Math.random() - 0.5) * 10 - 2;
      }
    }
    geometry.attributes.position.needsUpdate = true;

    points.rotation.y += 0.0002;
    renderer.render(scene, camera);
  }
  animate();

  window.addEventListener('resize', () => {
    camera.aspect = window.innerWidth / window.innerHeight;
    camera.updateProjectionMatrix();
    renderer.setSize(window.innerWidth, window.innerHeight);
  });
})();
