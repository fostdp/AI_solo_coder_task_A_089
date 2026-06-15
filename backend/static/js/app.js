import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';

const API_BASE = '/api';

const state = {
    currentSiteId: 1,
    sites: [],
    currentSite: null,
    simulation: null,
    weathering: null,
    sensorData: [],
    alarms: [],
    showStress: true,
    showCracks: true,
    showSensors: true,
    wireframe: false,
    stressMin: 0,
    stressMax: 25,
    renderMode: 'stress',
    chartTab: 'strain',
    scene: null,
    camera: null,
    renderer: null,
    controls: null,
    plankRoadGroup: null,
    woodMaterial: null,
    rockMaterial: null,
    sensorMarkers: [],
    crackMeshes: [],
};

const rockTextures = {
    '石灰岩': { base: '#6b7a8a', rough: 0.9, detail: [[-10, -5, -8], [8, -12, -3], [-5, 3, -15]] },
    '花岗岩': { base: '#d4d4d4', rough: 0.85, detail: [[-8, -6, -6], [10, -10, -10], [-3, 5, -12]] },
    '片麻岩': { base: '#8a8577', rough: 0.8, detail: [[-12, -4, -7], [6, -14, -5], [-7, 2, -18]] },
    '大理岩': { base: '#f0ebe3', rough: 0.6, detail: [[-6, -8, -9], [9, -6, -11], [-4, 4, -14]] },
    '砂岩':   { base: '#c89b7b', rough: 0.95, detail: [[-15, -3, -5], [12, -8, -8], [-6, 6, -16]] },
    '板岩':   { base: '#5a5a5a', rough: 0.88, detail: [[-9, -7, -6], [7, -13, -9], [-5, 4, -17]] },
};

const woodColors = {
    '柏木':  { base: '#8B5A2B', grain: '#6B4423', rough: 0.7 },
    '青冈木':{ base: '#A0522D', grain: '#7B3F1A', rough: 0.65 },
    '松木':  { base: '#DEB887', grain: '#B8956A', rough: 0.75 },
    '栎木':  { base: '#B8860B', grain: '#8B6508', rough: 0.6 },
    '杉木':  { base: '#D2B48C', grain: '#B89968', rough: 0.78 },
};

const gradeColors = {
    'SLIGHT':   '#10b981',
    'MILD':     '#22c55e',
    'MODERATE': '#f59e0b',
    'SERIOUS':  '#f97316',
    'SEVERE':   '#ef4444',
};

function init() {
    initClock();
    initThree();
    setupEventListeners();
    loadDashboard();
    setInterval(updateClock, 1000);
    setInterval(loadDashboard, 30000);

    setTimeout(() => {
        document.getElementById('loadingOverlay').classList.add('hidden');
    }, 1000);
}

function initClock() {
    updateClock();
}

function updateClock() {
    const now = new Date();
    document.getElementById('clock').textContent = now.toLocaleTimeString('zh-CN', { hour12: false });
}

function initThree() {
    const container = document.getElementById('canvasContainer');
    const canvas = document.getElementById('threeCanvas');

    state.scene = new THREE.Scene();
    state.scene.background = null;
    state.scene.fog = new THREE.Fog(0x0a1628, 80, 300);

    state.camera = new THREE.PerspectiveCamera(55, container.clientWidth / container.clientHeight, 0.1, 1000);
    state.camera.position.set(35, 25, 45);

    state.renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true });
    state.renderer.setSize(container.clientWidth, container.clientHeight);
    state.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
    state.renderer.shadowMap.enabled = true;
    state.renderer.shadowMap.type = THREE.PCFSoftShadowMap;
    state.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    state.renderer.toneMappingExposure = 1.1;

    state.controls = new OrbitControls(state.camera, state.renderer.domElement);
    state.controls.enableDamping = true;
    state.controls.dampingFactor = 0.08;
    state.controls.minDistance = 10;
    state.controls.maxDistance = 200;
    state.controls.maxPolarAngle = Math.PI / 2.1;
    state.controls.target.set(0, 5, 0);

    setupLights();
    setupEnvironment();
    state.plankRoadGroup = new THREE.Group();
    state.scene.add(state.plankRoadGroup);

    window.addEventListener('resize', onWindowResize);
    animate();
}

function setupLights() {
    const ambient = new THREE.AmbientLight(0x607d9c, 0.5);
    state.scene.add(ambient);

    const hemiLight = new THREE.HemisphereLight(0x87ceeb, 0x3a5f3a, 0.4);
    state.scene.add(hemiLight);

    const sunLight = new THREE.DirectionalLight(0xfff5e6, 1.2);
    sunLight.position.set(60, 80, 40);
    sunLight.castShadow = true;
    sunLight.shadow.mapSize.set(2048, 2048);
    sunLight.shadow.camera.left = -80;
    sunLight.shadow.camera.right = 80;
    sunLight.shadow.camera.top = 80;
    sunLight.shadow.camera.bottom = -80;
    sunLight.shadow.camera.near = 0.5;
    sunLight.shadow.camera.far = 300;
    state.scene.add(sunLight);

    const fillLight = new THREE.DirectionalLight(0x4488ff, 0.3);
    fillLight.position.set(-40, 30, -20);
    state.scene.add(fillLight);

    const warmLight = new THREE.PointLight(0xffaa55, 0.4, 100);
    warmLight.position.set(0, 15, 20);
    state.scene.add(warmLight);
}

function setupEnvironment() {
    const groundGeo = new THREE.PlaneGeometry(400, 400, 64, 64);
    const positions = groundGeo.attributes.position;
    for (let i = 0; i < positions.count; i++) {
        const x = positions.getX(i);
        const y = positions.getY(i);
        const z = Math.sin(x * 0.05) * 3 + Math.cos(y * 0.07) * 2 + Math.random() * 0.8;
        positions.setZ(i, z);
    }
    groundGeo.computeVertexNormals();

    const groundMat = new THREE.MeshStandardMaterial({
        color: 0x3a5f3a,
        roughness: 0.95,
        metalness: 0.0,
    });
    const ground = new THREE.Mesh(groundGeo, groundMat);
    ground.rotation.x = -Math.PI / 2;
    ground.receiveShadow = true;
    state.scene.add(ground);

    for (let i = 0; i < 15; i++) {
        const angle = Math.random() * Math.PI * 2;
        const dist = 60 + Math.random() * 80;
        const x = Math.cos(angle) * dist;
        const z = Math.sin(angle) * dist;
        const scale = 0.5 + Math.random() * 1.5;
        createMountain(x, -5 + Math.random() * 3, z, scale);
    }

    for (let i = 0; i < 80; i++) {
        const angle = Math.random() * Math.PI * 2;
        const dist = 40 + Math.random() * 100;
        const x = Math.cos(angle) * dist;
        const z = Math.sin(angle) * dist;
        if (Math.abs(x) < 35 && Math.abs(z) < 25) continue;
        const s = 0.5 + Math.random() * 1.5;
        createTree(x, 0, z, s);
    }
}

function createMountain(x, y, z, scale) {
    const geo = new THREE.ConeGeometry(15 * scale, 40 * scale, 8, 4);
    const mat = new THREE.MeshStandardMaterial({
        color: new THREE.Color().setHSL(0.58, 0.1, 0.3 + Math.random() * 0.1),
        roughness: 0.9,
        flatShading: true,
    });
    const mountain = new THREE.Mesh(geo, mat);
    mountain.position.set(x, y + 20 * scale, z);
    mountain.castShadow = true;
    mountain.receiveShadow = true;
    state.scene.add(mountain);

    const snowGeo = new THREE.ConeGeometry(10 * scale, 12 * scale, 8);
    const snowMat = new THREE.MeshStandardMaterial({
        color: 0xf0f8ff,
        roughness: 0.4,
        flatShading: true,
    });
    const snow = new THREE.Mesh(snowGeo, snowMat);
    snow.position.set(x, y + 38 * scale, z);
    state.scene.add(snow);
}

function createTree(x, y, z, scale) {
    const trunkGeo = new THREE.CylinderGeometry(0.15 * scale, 0.25 * scale, 3 * scale, 6);
    const trunkMat = new THREE.MeshStandardMaterial({ color: 0x4a3728, roughness: 0.9 });
    const trunk = new THREE.Mesh(trunkGeo, trunkMat);
    trunk.position.set(x, y + 1.5 * scale, z);
    trunk.castShadow = true;
    state.scene.add(trunk);

    const coneGeo = new THREE.ConeGeometry(1.5 * scale, 4 * scale, 7);
    const coneMat = new THREE.MeshStandardMaterial({
        color: new THREE.Color().setHSL(0.28 + Math.random() * 0.05, 0.5, 0.25 + Math.random() * 0.1),
        roughness: 0.85,
    });
    const cone = new THREE.Mesh(coneGeo, coneMat);
    cone.position.set(x, y + 4 * scale, z);
    cone.castShadow = true;
    state.scene.add(cone);
}

function buildPlankRoad(site) {
    clearGroup(state.plankRoadGroup);
    state.sensorMarkers = [];
    state.crackMeshes = [];

    const rockType = site.RockType || '石灰岩';
    const woodType = site.WoodType || '柏木';
    const rockCfg = rockTextures[rockType] || rockTextures['石灰岩'];
    const woodCfg = woodColors[woodType] || woodColors['柏木'];

    const beamCount = Math.min(site.BeamCount || 50, 80);
    const totalLength = Math.min(site.TotalLength || 50, 100);
    const beamSpacing = totalLength / beamCount;
    const mountainHeight = 15;

    buildRockMountain(rockCfg, totalLength, mountainHeight);
    buildBeamsAndPlanks(woodCfg, beamCount, beamSpacing);
    buildBeamHoles(rockCfg, beamCount, beamSpacing, mountainHeight);

    if (state.showCracks) {
        buildCracks(rockCfg, totalLength, mountainHeight);
    }

    if (state.showSensors) {
        buildSensorMarkers(beamCount, beamSpacing);
    }

    applyStressColors();
    updateStatsPanel();
}

function clearGroup(group) {
    while (group.children.length > 0) {
        const child = group.children[0];
        if (child.geometry) child.geometry.dispose();
        if (child.material) {
            if (Array.isArray(child.material)) {
                child.material.forEach(m => m.dispose());
            } else {
                child.material.dispose();
            }
        }
        group.remove(child);
    }
}

function buildRockMountain(cfg, length, height) {
    const cliffGeo = new THREE.BoxGeometry(length + 20, height, 20);
    const positions = cliffGeo.attributes.position;
    for (let i = 0; i < positions.count; i++) {
        const z = positions.getZ(i);
        if (z < -5) {
            positions.setX(i, positions.getX(i) + (Math.random() - 0.5) * 3);
            positions.setY(i, positions.getY(i) + (Math.random() - 0.5) * 4);
        }
    }
    cliffGeo.computeVertexNormals();

    const canvas = document.createElement('canvas');
    canvas.width = 1024;
    canvas.height = 1024;
    const ctx = canvas.getContext('2d');
    generateRockTexture(ctx, cfg, 1024);
    const texture = new THREE.CanvasTexture(canvas);
    texture.wrapS = texture.wrapT = THREE.RepeatWrapping;
    texture.repeat.set(4, 3);

    state.rockMaterial = new THREE.MeshStandardMaterial({
        map: texture,
        roughness: cfg.rough,
        metalness: 0.05,
    });

    const cliff = new THREE.Mesh(cliffGeo, state.rockMaterial);
    cliff.position.set(0, height / 2 - 2, -10);
    cliff.castShadow = true;
    cliff.receiveShadow = true;
    state.plankRoadGroup.add(cliff);

    for (let i = 0; i < 30; i++) {
        const rockSize = 0.5 + Math.random() * 2.5;
        const rockGeo = new THREE.DodecahedronGeometry(rockSize, 0);
        const rockPos = rockGeo.attributes.position;
        for (let j = 0; j < rockPos.count; j++) {
            rockPos.setX(j, rockPos.getX(j) * (0.8 + Math.random() * 0.4));
            rockPos.setY(j, rockPos.getY(j) * (0.8 + Math.random() * 0.4));
            rockPos.setZ(j, rockPos.getZ(j) * (0.8 + Math.random() * 0.4));
        }
        rockGeo.computeVertexNormals();

        const rockMat = state.rockMaterial.clone();
        const rock = new THREE.Mesh(rockGeo, rockMat);
        rock.position.set(
            (Math.random() - 0.5) * (length + 15),
            Math.random() * (height - 2) - 1,
            -10 + (Math.random() - 0.5) * 15
        );
        rock.rotation.set(Math.random() * Math.PI, Math.random() * Math.PI, Math.random() * Math.PI);
        rock.castShadow = true;
        rock.receiveShadow = true;
        rock.userData.isRock = true;
        state.plankRoadGroup.add(rock);
    }
}

function generateRockTexture(ctx, cfg, size) {
    const baseColor = cfg.base;
    ctx.fillStyle = baseColor;
    ctx.fillRect(0, 0, size, size);

    const base = new THREE.Color(baseColor);

    for (let i = 0; i < 15000; i++) {
        const x = Math.random() * size;
        const y = Math.random() * size;
        const r = Math.random() * 3 + 0.5;
        const shade = Math.random() * 0.3 - 0.15;
        const c = base.clone();
        c.offsetHSL(0, 0, shade);
        ctx.fillStyle = `rgb(${Math.floor(c.r*255)}, ${Math.floor(c.g*255)}, ${Math.floor(c.b*255)})`;
        ctx.beginPath();
        ctx.arc(x, y, r, 0, Math.PI * 2);
        ctx.fill();
    }

    ctx.strokeStyle = 'rgba(0,0,0,0.2)';
    ctx.lineWidth = 1;
    for (let i = 0; i < 80; i++) {
        ctx.beginPath();
        ctx.moveTo(Math.random() * size, Math.random() * size);
        ctx.bezierCurveTo(
            Math.random() * size, Math.random() * size,
            Math.random() * size, Math.random() * size,
            Math.random() * size, Math.random() * size
        );
        ctx.stroke();
    }

    for (let i = 0; i < 200; i++) {
        const x = Math.random() * size;
        const y = Math.random() * size;
        const w = 20 + Math.random() * 80;
        const h = 2 + Math.random() * 8;
        const angle = Math.random() * Math.PI;
        ctx.save();
        ctx.translate(x, y);
        ctx.rotate(angle);
        ctx.fillStyle = `rgba(0,0,0,${0.05 + Math.random()*0.1})`;
        ctx.fillRect(-w/2, -h/2, w, h);
        ctx.restore();
    }

    for (let i = 0; i < 400; i++) {
        const x = Math.random() * size;
        const y = Math.random() * size;
        ctx.fillStyle = `rgba(139,90,43,${Math.random() * 0.1})`;
        ctx.beginPath();
        ctx.arc(x, y, Math.random() * 4, 0, Math.PI * 2);
        ctx.fill();
    }
}

function buildBeamsAndPlanks(woodCfg, beamCount, beamSpacing) {
    const canvas = document.createElement('canvas');
    canvas.width = 512;
    canvas.height = 512;
    const ctx = canvas.getContext('2d');
    generateWoodTexture(ctx, woodCfg, 512);
    const woodTex = new THREE.CanvasTexture(canvas);
    woodTex.wrapS = woodTex.wrapT = THREE.RepeatWrapping;
    woodTex.repeat.set(1, 2);

    state.woodMaterial = new THREE.MeshStandardMaterial({
        map: woodTex,
        roughness: woodCfg.rough,
        metalness: 0.02,
    });

    for (let i = 0; i < beamCount; i++) {
        const x = -beamSpacing * beamCount / 2 + i * beamSpacing;
        const beamGeo = new THREE.BoxGeometry(0.2, 0.15, 2.5);
        const beamMat = state.woodMaterial.clone();
        const beam = new THREE.Mesh(beamGeo, beamMat);
        beam.position.set(x, 5, -5.5);
        beam.rotation.x = -Math.PI / 2;
        beam.castShadow = true;
        beam.receiveShadow = true;
        beam.userData = { isWood: true, isBeam: true, beamIndex: i, baseStress: 8 + Math.random() * 15 };
        state.plankRoadGroup.add(beam);
    }

    const plankCount = Math.floor(beamCount * 3);
    const plankSpacing = (beamSpacing * beamCount) / plankCount;
    for (let i = 0; i < plankCount; i++) {
        const x = -beamSpacing * beamCount / 2 + plankSpacing * i + plankSpacing / 2;
        const plankGeo = new THREE.BoxGeometry(plankSpacing * 0.95, 0.04, 1.8);
        const plankMat = state.woodMaterial.clone();
        const plank = new THREE.Mesh(plankGeo, plankMat);
        plank.position.set(x, 5.08, -5.5);
        plank.castShadow = true;
        plank.receiveShadow = true;
        plank.userData = { isWood: true, isPlank: true, baseStress: 3 + Math.random() * 8 };
        state.plankRoadGroup.add(plank);
    }

    for (let i = 0; i < beamCount; i += 5) {
        const x = -beamSpacing * beamCount / 2 + i * beamSpacing;
        const postGeo = new THREE.CylinderGeometry(0.08, 0.1, 5.5, 8);
        const postMat = state.woodMaterial.clone();
        const post = new THREE.Mesh(postGeo, postMat);
        post.position.set(x, 2.25, -6.5);
        post.castShadow = true;
        post.userData = { isWood: true, isPost: true, baseStress: 5 + Math.random() * 10 };
        state.plankRoadGroup.add(post);
    }
}

function generateWoodTexture(ctx, cfg, size) {
    ctx.fillStyle = cfg.base;
    ctx.fillRect(0, 0, size, size);

    const base = new THREE.Color(cfg.base);
    for (let y = 0; y < size; y += 2) {
        const variance = Math.sin(y * 0.08) * 10 + Math.sin(y * 0.2) * 5 + Math.random() * 3;
        const c = base.clone();
        c.offsetHSL(0, 0, variance / 200);
        ctx.fillStyle = `rgb(${Math.floor(c.r*255)}, ${Math.floor(c.g*255)}, ${Math.floor(c.b*255)})`;
        ctx.fillRect(0, y, size, 2);
    }

    ctx.strokeStyle = cfg.grain;
    ctx.lineWidth = 1;
    for (let i = 0; i < 40; i++) {
        ctx.beginPath();
        const yStart = Math.random() * size;
        ctx.moveTo(0, yStart);
        for (let x = 0; x < size; x += 20) {
            const y = yStart + Math.sin(x * 0.01 + i) * 8 + Math.random() * 3;
            ctx.lineTo(x, y);
        }
        ctx.globalAlpha = 0.1 + Math.random() * 0.15;
        ctx.stroke();
    }
    ctx.globalAlpha = 1;

    for (let i = 0; i < 5; i++) {
        const x = Math.random() * size;
        const y = Math.random() * size;
        const r = 3 + Math.random() * 8;
        ctx.fillStyle = cfg.grain;
        ctx.globalAlpha = 0.2;
        ctx.beginPath();
        ctx.arc(x, y, r, 0, Math.PI * 2);
        ctx.fill();
        for (let ring = 1; ring < 6; ring++) {
            ctx.beginPath();
            ctx.arc(x, y, r + ring * 2, 0, Math.PI * 2);
            ctx.globalAlpha = 0.1 / ring;
            ctx.stroke();
        }
    }
    ctx.globalAlpha = 1;
}

function buildBeamHoles(rockCfg, beamCount, beamSpacing, mountainHeight) {
    for (let i = 0; i < beamCount; i++) {
        const x = -beamSpacing * beamCount / 2 + i * beamSpacing;
        const holeGeo = new THREE.BoxGeometry(0.28, 0.22, 0.8);
        const holeMat = new THREE.MeshStandardMaterial({
            color: 0x1a1a1a,
            roughness: 0.95,
        });
        const hole = new THREE.Mesh(holeGeo, holeMat);
        hole.position.set(x, 5, -9.3);
        state.plankRoadGroup.add(hole);

        const ringGeo = new THREE.TorusGeometry(0.2, 0.02, 6, 12);
        const ringMat = new THREE.MeshStandardMaterial({
            color: new THREE.Color(rockCfg.base).offsetHSL(0, 0, -0.2),
            roughness: 0.9,
        });
        const ring = new THREE.Mesh(ringGeo, ringMat);
        ring.position.set(x, 5, -8.9);
        ring.rotation.y = Math.PI / 2;
        state.plankRoadGroup.add(ring);
    }
}

function buildCracks(rockCfg, length, height) {
    state.crackMeshes = [];

    for (let i = 0; i < 35; i++) {
        const depth = state.weathering ? state.weathering.CurrentCrackDepth / 50 : 0.3;
        const points = [];
        const startX = -length / 2 + Math.random() * length;
        const startY = Math.random() * (height - 2);
        let x = startX, y = startY;

        for (let t = 0; t < 30; t++) {
            points.push(new THREE.Vector3(x, y, -0.5));
            x += (Math.random() - 0.3) * 2;
            y += (Math.random() - 0.5) * 2;
            if (y < 0 || y > height) break;
        }

        const curve = new THREE.CatmullRomCurve3(points);
        const crackGeo = new THREE.TubeGeometry(curve, points.length, 0.02 + depth * 0.05, 6, false);
        const crackMat = new THREE.MeshBasicMaterial({
            color: 0x0a0a0a,
            transparent: true,
            opacity: 0.85,
        });
        const crack = new THREE.Mesh(crackGeo, crackMat);
        crack.position.z = -10;
        state.plankRoadGroup.add(crack);
        state.crackMeshes.push(crack);

        if (state.weathering && i % 5 === 0) {
            const widen = depth > 0.06;
            if (widen) {
                const wideCurve = new THREE.CatmullRomCurve3(points);
                const wideGeo = new THREE.TubeGeometry(wideCurve, points.length, 0.08 + depth * 0.1, 4, false);
                const wideMat = new THREE.MeshBasicMaterial({
                    color: 0x1a1510,
                    transparent: true,
                    opacity: 0.6,
                });
                const wideCrack = new THREE.Mesh(wideGeo, wideMat);
                wideCrack.position.z = -10;
                state.plankRoadGroup.add(wideCrack);
                state.crackMeshes.push(wideCrack);
            }
        }
    }
}

function buildSensorMarkers(beamCount, beamSpacing) {
    state.sensorMarkers = [];

    for (let i = 0; i < beamCount; i += Math.max(1, Math.floor(beamCount / 20))) {
        const x = -beamSpacing * beamCount / 2 + i * beamSpacing;

        const positions = [
            { x: x, y: 5.05, z: -4.5, type: 'STRAIN' },
            { x: x, y: 4.95, z: -6.5, type: 'STRAIN' },
            { x: x, y: 5,    z: -8.5, type: 'CRACK'  },
        ];

        positions.forEach((pos, idx) => {
            const color = pos.type === 'STRAIN' ? 0x00d4ff : 0xff6b35;
            const geo = new THREE.SphereGeometry(0.1, 12, 12);
            const mat = new THREE.MeshStandardMaterial({
                color,
                emissive: color,
                emissiveIntensity: 0.5,
                roughness: 0.3,
                metalness: 0.5,
            });
            const marker = new THREE.Mesh(geo, mat);
            marker.position.set(pos.x, pos.y, pos.z);
            marker.userData = { isSensor: true, type: pos.type, beamIdx: i, sensorIdx: idx };
            state.plankRoadGroup.add(marker);
            state.sensorMarkers.push(marker);

            const ringGeo = new THREE.RingGeometry(0.15, 0.2, 16);
            const ringMat = new THREE.MeshBasicMaterial({
                color,
                side: THREE.DoubleSide,
                transparent: true,
                opacity: 0.6,
            });
            const ring = new THREE.Mesh(ringGeo, ringMat);
            ring.position.copy(marker.position);
            ring.rotation.x = -Math.PI / 2;
            state.plankRoadGroup.add(ring);

            const glow = { mesh: ring, base: 0.15, phase: Math.random() * Math.PI * 2 };
            state.sensorMarkers.push(glow);
        });
    }
}

function applyStressColors() {
    let maxWood = 0, maxRock = 0;
    let minWood = Infinity, minRock = Infinity;

    state.plankRoadGroup.traverse((obj) => {
        if (!obj.isMesh) return;
        const ud = obj.userData;
        let stress = 0;

        if (ud.isWood) {
            stress = ud.baseStress || 10;
            if (state.simulation) {
                const factor = (state.simulation.MaxWoodStress || 10) / 10;
                stress *= (0.5 + factor * (0.5 + Math.random() * 0.5));
            }
            ud.computedStress = stress;
            maxWood = Math.max(maxWood, stress);
            minWood = Math.min(minWood, stress);
        }
        if (ud.isRock) {
            stress = 5 + Math.random() * 15;
            if (state.simulation) {
                const factor = (state.simulation.MaxRockStress || 10) / 10;
                stress *= (0.4 + factor * (0.6 + Math.random() * 0.4));
            }
            ud.computedStress = stress;
            maxRock = Math.max(maxRock, stress);
            minRock = Math.min(minRock, stress);
        }
    });

    state.plankRoadGroup.traverse((obj) => {
        if (!obj.isMesh || !obj.material || obj.material.map) return;
        if (obj.userData.isSensor) return;

        const stress = obj.userData.computedStress || 0;
        let color;

        if (state.renderMode === 'material') {
            return;
        } else if (state.renderMode === 'weathering') {
            const grade = state.weathering?.WeatheringGrade || 'MODERATE';
            color = new THREE.Color(gradeColors[grade] || gradeColors.MODERATE);
            if (obj.userData.isWood) {
                const grade = state.weathering?.WeatheringGrade || 'MODERATE';
                color = new THREE.Color(gradeColors[grade] || gradeColors.MODERATE);
            }
        } else {
            color = stressToColor(stress, state.stressMin, state.stressMax);
        }

        if (state.showStress || state.renderMode !== 'stress') {
            if (obj.material.color && !obj.material.map) {
                obj.material.color = color;
            }
        }

        if (state.wireframe) {
            obj.material.wireframe = true;
        }
    });
}

function stressToColor(stress, min, max) {
    const t = Math.max(0, Math.min(1, (stress - min) / (max - min)));
    const colors = [
        [0.0627, 0.7255, 0.5059],
        [0.2314, 0.5098, 0.9647],
        [0.5451, 0.3608, 0.9647],
        [0.9608, 0.6196, 0.0431],
        [0.9765, 0.4510, 0.0863],
        [0.9373, 0.2667, 0.2667],
    ];

    const idx = t * (colors.length - 1);
    const i = Math.floor(idx);
    const f = idx - i;
    const c1 = colors[Math.min(i, colors.length - 1)];
    const c2 = colors[Math.min(i + 1, colors.length - 1)];

    return new THREE.Color(
        c1[0] + (c2[0] - c1[0]) * f,
        c1[1] + (c2[1] - c1[1]) * f,
        c1[2] + (c2[2] - c1[2]) * f
    );
}

function updateStatsPanel() {
    if (state.simulation) {
        document.getElementById('maxWoodStress').textContent = `${state.simulation.MaxWoodStress?.toFixed(2) || '--'} MPa`;
        document.getElementById('maxRockStress').textContent = `${state.simulation.MaxRockStress?.toFixed(2) || '--'} MPa`;
        document.getElementById('maxDeflection').textContent = `${state.simulation.MaxDeflectionMM?.toFixed(3) || '--'} mm`;
        const sf = state.simulation.SafetyFactor;
        const sfEl = document.getElementById('safetyFactor');
        sfEl.textContent = sf ? sf.toFixed(2) : '--';
        sfEl.style.color = sf < 1.2 ? 'var(--accent-red)' : sf < 1.5 ? 'var(--accent-yellow)' : 'var(--accent-green)';
    }
}

async function loadDashboard() {
    try {
        const res = await fetch(`${API_BASE}/dashboard`);
        const data = await res.json();
        if (data.code === 0) {
            renderSiteList(data.data.site_statuses);
            const conn = document.querySelector('.connection-status');
            conn.querySelector('.status-dot').classList.add('connected');
            conn.querySelector('span:last-child').textContent = '已连接';
        }
    } catch (e) {
        console.warn('Dashboard load failed:', e);
    }

    try {
        const alarmRes = await fetch(`${API_BASE}/alarms`);
        const alarmData = await alarmRes.json();
        if (alarmData.code === 0) {
            state.alarms = alarmData.data;
            renderAlarms();
        }
    } catch (e) {}

    if (state.currentSite) {
        loadSensorData(state.currentSiteId);
    }
}

function renderSiteList(statuses) {
    state.sites = statuses;
    const container = document.getElementById('siteList');

    if (!statuses || statuses.length === 0) {
        container.innerHTML = '<div class="loading">暂无遗址数据</div>';
        return;
    }

    container.innerHTML = '';
    statuses.forEach((s) => {
        const div = document.createElement('div');
        div.className = 'site-item' + (s.site_id === state.currentSiteId ? ' active' : '');
        div.innerHTML = `
            <div class="site-item-header">
                <span class="site-name">${s.site_name}</span>
                <span class="status-badge status-${s.status.toLowerCase()}">${s.status}</span>
            </div>
            <div class="site-region">${s.region} · 遗址ID:${s.site_id}</div>
            <div class="site-meta">
                <span class="site-stat">🔔 ${s.alarm_count}</span>
                ${s.safety_factor > 0 ? `<span class="site-stat">🛡️ ${s.safety_factor.toFixed(2)}</span>` : ''}
                ${s.predicted_lifespan > 0 ? `<span class="site-stat">⏳ ${s.predicted_lifespan.toFixed(0)}年</span>` : ''}
            </div>
        `;
        div.addEventListener('click', () => selectSite(s.site_id));
        container.appendChild(div);
    });
}

async function selectSite(siteId) {
    state.currentSiteId = siteId;
    document.querySelectorAll('.site-item').forEach(el => el.classList.remove('active'));
    document.querySelectorAll('.site-item').forEach((el, i) => {
        if (state.sites[i]?.site_id === siteId) el.classList.add('active');
    });

    try {
        const res = await fetch(`${API_BASE}/sites/${siteId}`);
        const data = await res.json();
        if (data.code === 0) {
            state.currentSite = data.data;
            renderSiteInfo(data.data);
            buildPlankRoad(data.data);
        }
    } catch (e) {}

    try {
        const simRes = await fetch(`${API_BASE}/sites/${siteId}/simulation`);
        const simData = await simRes.json();
        if (simData.code === 0) state.simulation = simData.data;
    } catch (e) {}

    try {
        const wRes = await fetch(`${API_BASE}/sites/${siteId}/weathering`);
        const wData = await wRes.json();
        if (wData.code === 0) state.weathering = wData.data;
        renderWeathering(state.weathering);
    } catch (e) {}

    applyStressColors();
    loadSensorData(siteId);
}

function renderSiteInfo(site) {
    document.getElementById('siteInfoPanel').style.display = 'block';
    const info = document.getElementById('siteInfo');
    info.innerHTML = `
        <div class="info-item"><span class="info-label">名称</span><span class="info-value">${site.site_name}</span></div>
        <div class="info-item"><span class="info-label">地区</span><span class="info-value">${site.region}</span></div>
        <div class="info-item"><span class="info-label">海拔</span><span class="info-value">${site.elevation} m</span></div>
        <div class="info-item"><span class="info-label">年代</span><span class="info-value">${site.construction_era}</span></div>
        <div class="info-item"><span class="info-label">长度</span><span class="info-value">${site.total_length} m</span></div>
        <div class="info-item"><span class="info-label">梁数</span><span class="info-value">${site.beam_count}</span></div>
        <div class="info-item"><span class="info-label">岩体</span><span class="info-value">${site.rock_type}</span></div>
        <div class="info-item"><span class="info-label">木材</span><span class="info-value">${site.wood_type}</span></div>
    `;
}

function renderWeathering(w) {
    document.getElementById('weatheringPanel').style.display = 'block';
    const el = document.getElementById('weatheringInfo');
    if (!w) {
        el.innerHTML = '<div class="loading">暂无风化评估数据，请点击"风化评估"按钮</div>';
        return;
    }

    const grade = w.weathering_grade || 'MODERATE';
    const gradeText = { SLIGHT:'轻微', MILD:'轻度', MODERATE:'中度', SERIOUS:'严重', SEVERE:'极重' }[grade] || '中度';
    const lifePercent = w.predicted_lifespan ? Math.min(100, (w.remaining_lifespan / w.predicted_lifespan) * 100) : 50;
    const lifeColor = lifePercent < 20 ? 'var(--accent-red)' : lifePercent < 50 ? 'var(--accent-yellow)' : 'var(--accent-green)';

    el.innerHTML = `
        <div class="weathering-grade grade-${grade}">风化等级: ${gradeText}</div>
        <div class="weathering-detail">
            <div class="detail-row"><span class="detail-label">冻融循环</span><span class="detail-value">${w.freeze_thaw_cycles} 次</span></div>
            <div class="detail-row"><span class="detail-label">当前裂隙深</span><span class="detail-value">${w.current_crack_depth?.toFixed(2) || '--'} mm</span></div>
            <div class="detail-row"><span class="detail-label">年扩展率</span><span class="detail-value">${w.crack_propagation_rate?.toFixed(4) || '--'} mm/年</span></div>
            <div class="detail-row"><span class="detail-label">木材腐朽率</span><span class="detail-value">${w.wood_decay_rate?.toFixed(4) || '--'} %/年</span></div>
            <div class="detail-row"><span class="detail-label">岩体侵蚀率</span><span class="detail-value">${w.rock_erosion_rate?.toFixed(4) || '--'} mm/年</span></div>
            <div class="detail-row"><span class="detail-label">预测寿命</span><span class="detail-value">${w.predicted_lifespan?.toFixed(1) || '--'} 年</span></div>
            <div class="detail-row"><span class="detail-label">剩余寿命</span><span class="detail-value" style="color:${lifeColor}">${w.remaining_lifespan?.toFixed(1) || '--'} 年</span></div>
            <div class="detail-row"><span class="detail-label">置信度</span><span class="detail-value">${(w.confidence*100)?.toFixed(0) || '--'}%</span></div>
        </div>
        <div class="lifespan-bar">
            <div class="lifespan-fill" style="width:${lifePercent}%; background:${lifeColor}"></div>
        </div>
    `;
}

async function loadSensorData(siteId) {
    try {
        const res = await fetch(`${API_BASE}/sensor?site_id=${siteId}&hours=72&limit=1000`);
        const data = await res.json();
        if (data.code === 0) {
            state.sensorData = data.data;
            drawChart();
        }
    } catch (e) {}
}

function renderAlarms() {
    const container = document.getElementById('alarmList');
    if (!state.alarms || state.alarms.length === 0) {
        container.innerHTML = '<div class="no-alarm">✅ 系统运行正常，暂无告警</div>';
        return;
    }

    const siteNames = {};
    state.sites.forEach(s => siteNames[s.site_id] = s.site_name);

    container.innerHTML = '';
    state.alarms.slice(0, 50).forEach(a => {
        const div = document.createElement('div');
        div.className = `alarm-item level-${a.alarm_level}`;
        div.innerHTML = `
            <div class="alarm-header">
                <span class="alarm-type ${a.alarm_type}">${a.alarm_type === 'STRAIN' ? '梁孔应变' : '岩体裂隙'}</span>
                <span class="alarm-level">${a.alarm_level === 'WARNING' ? '警告' : '严重'}</span>
            </div>
            <div class="alarm-site">📍 ${siteNames[a.site_id] || '遗址#'+a.site_id} ${a.beam_id ? `· 梁#${a.beam_id}` : ''}</div>
            <div class="alarm-desc">${a.description}</div>
            <div class="alarm-time">⏰ ${new Date(a.time).toLocaleString('zh-CN')}</div>
        `;
        container.appendChild(div);
    });
}

function drawChart() {
    const canvas = document.getElementById('chartCanvas');
    const container = canvas.parentElement;
    canvas.width = container.clientWidth;
    canvas.height = container.clientHeight;
    const ctx = canvas.getContext('2d');

    const data = state.sensorData || [];
    if (data.length === 0) {
        ctx.fillStyle = '#6c7a89';
        ctx.font = '14px sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText('暂无监测数据', canvas.width / 2, canvas.height / 2);
        return;
    }

    const sorted = [...data].sort((a, b) => new Date(a.time) - new Date(b.time));
    const W = canvas.width, H = canvas.height;
    const pad = { l: 50, r: 20, t: 20, b: 40 };
    const cw = W - pad.l - pad.r;
    const ch = H - pad.t - pad.b;

    let series;
    const tab = state.chartTab;

    if (tab === 'strain') {
        series = [
            { label: '顶部应变 (με)', color: '#8b5cf6', key: 'beam_strain_top' },
            { label: '底部应变 (με)', color: '#06b6d4', key: 'beam_strain_bottom' },
            { label: '侧部应变 (με)', color: '#f59e0b', key: 'beam_strain_side' },
        ];
    } else if (tab === 'crack') {
        series = [
            { label: '裂隙1 (mm)', color: '#ef4444', key: 'rock_crack_width_1' },
            { label: '裂隙2 (mm)', color: '#f97316', key: 'rock_crack_width_2' },
            { label: '裂隙3 (mm)', color: '#f59e0b', key: 'rock_crack_width_3' },
        ];
    } else {
        series = [
            { label: '温度 (°C)', color: '#ef4444', key: 'temperature' },
            { label: '湿度 (%)', color: '#3b82f6', key: 'humidity', scale: 0.4 },
        ];
    }

    let minVal = Infinity, maxVal = -Infinity;
    series.forEach(s => {
        sorted.forEach(d => {
            const v = d[s.key] * (s.scale || 1);
            if (!isNaN(v)) { minVal = Math.min(minVal, v); maxVal = Math.max(maxVal, v); }
        });
    });
    const range = maxVal - minVal || 1;
    minVal -= range * 0.1;
    maxVal += range * 0.1;

    ctx.clearRect(0, 0, W, H);
    ctx.strokeStyle = 'rgba(45, 69, 96, 0.5)';
    ctx.lineWidth = 1;
    for (let i = 0; i <= 5; i++) {
        const y = pad.t + ch * i / 5;
        ctx.beginPath();
        ctx.moveTo(pad.l, y);
        ctx.lineTo(pad.l + cw, y);
        ctx.stroke();

        const val = maxVal - (maxVal - minVal) * i / 5;
        ctx.fillStyle = '#8fa3b8';
        ctx.font = '10px Courier New';
        ctx.textAlign = 'right';
        ctx.fillText(val.toFixed(1), pad.l - 6, y + 3);
    }

    const n = sorted.length;
    for (let i = 0; i < n; i += Math.max(1, Math.floor(n / 6))) {
        const x = pad.l + cw * i / (n - 1 || 1);
        ctx.beginPath();
        ctx.moveTo(x, pad.t);
        ctx.lineTo(x, pad.t + ch);
        ctx.stroke();

        const t = new Date(sorted[i].time);
        ctx.fillStyle = '#8fa3b8';
        ctx.textAlign = 'center';
        ctx.fillText(t.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }), x, pad.t + ch + 16);
    }

    series.forEach(s => {
        ctx.strokeStyle = s.color;
        ctx.lineWidth = 2;
        ctx.beginPath();
        let started = false;
        sorted.forEach((d, i) => {
            const v = d[s.key] * (s.scale || 1);
            if (isNaN(v)) return;
            const x = pad.l + cw * i / (n - 1 || 1);
            const y = pad.t + ch * (1 - (v - minVal) / (maxVal - minVal));
            if (!started) { ctx.moveTo(x, y); started = true; }
            else ctx.lineTo(x, y);
        });
        ctx.stroke();

        ctx.fillStyle = s.color;
        sorted.forEach((d, i) => {
            const v = d[s.key] * (s.scale || 1);
            if (isNaN(v)) return;
            const x = pad.l + cw * i / (n - 1 || 1);
            const y = pad.t + ch * (1 - (v - minVal) / (maxVal - minVal));
            ctx.beginPath();
            ctx.arc(x, y, 2.5, 0, Math.PI * 2);
            ctx.fill();
        });
    });

    ctx.font = '11px sans-serif';
    ctx.textAlign = 'left';
    series.forEach((s, i) => {
        const x = pad.l + 10 + i * 160;
        const y = pad.t + ch + 32;
        ctx.fillStyle = s.color;
        ctx.fillRect(x, y, 12, 3);
        ctx.fillStyle = '#e8edf2';
        ctx.fillText(s.label, x + 18, y + 4);
    });
}

function setupEventListeners() {
    document.querySelectorAll('.tab-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            state.chartTab = btn.dataset.tab;
            drawChart();
        });
    });

    document.getElementById('showStress').addEventListener('change', (e) => {
        state.showStress = e.target.checked;
        applyStressColors();
    });

    document.getElementById('showCracks').addEventListener('change', (e) => {
        state.showCracks = e.target.checked;
        if (state.currentSite) buildPlankRoad(state.currentSite);
    });

    document.getElementById('showSensors').addEventListener('change', (e) => {
        state.showSensors = e.target.checked;
        if (state.currentSite) buildPlankRoad(state.currentSite);
    });

    document.getElementById('showWireframe').addEventListener('change', (e) => {
        state.wireframe = e.target.checked;
        state.plankRoadGroup.traverse(obj => {
            if (obj.isMesh && obj.material) obj.material.wireframe = state.wireframe;
        });
    });

    document.getElementById('stressMin').addEventListener('input', (e) => {
        state.stressMin = parseFloat(e.target.value);
        document.getElementById('stressMinVal').textContent = state.stressMin;
        applyStressColors();
    });

    document.getElementById('stressMax').addEventListener('input', (e) => {
        state.stressMax = parseFloat(e.target.value);
        document.getElementById('stressMaxVal').textContent = state.stressMax;
        applyStressColors();
    });

    document.getElementById('renderMode').addEventListener('change', (e) => {
        state.renderMode = e.target.value;
        applyStressColors();
    });

    document.getElementById('btnRefreshData').addEventListener('click', () => {
        loadSensorData(state.currentSiteId);
        loadDashboard();
        showToast('数据已刷新', 'success');
    });

    document.getElementById('btnSimulate').addEventListener('click', async () => {
        if (!state.currentSiteId) return showToast('请先选择遗址', 'warning');
        showToast('正在执行有限元仿真...', 'info');
        try {
            const res = await fetch(`${API_BASE}/sites/${state.currentSiteId}/simulate`, { method: 'POST' });
            const data = await res.json();
            if (data.code === 0) {
                state.simulation = data.data;
                applyStressColors();
                updateStatsPanel();
                showToast(`仿真完成! 安全系数=${data.data.safety_factor.toFixed(2)}`, 'success');
            }
        } catch (e) {
            showToast('仿真失败: ' + e.message, 'error');
        }
    });

    document.getElementById('btnWeathering').addEventListener('click', async () => {
        if (!state.currentSiteId) return showToast('请先选择遗址', 'warning');
        showToast('正在执行风化评估...', 'info');
        try {
            const res = await fetch(`${API_BASE}/sites/${state.currentSiteId}/weathering`, { method: 'POST' });
            const data = await res.json();
            if (data.code === 0) {
                state.weathering = data.data;
                renderWeathering(data.data);
                buildPlankRoad(state.currentSite);
                const gradeMap = { SLIGHT:'轻微', MILD:'轻度', MODERATE:'中度', SERIOUS:'严重', SEVERE:'极重' };
                showToast(`评估完成! 等级=${gradeMap[data.data.weathering_grade] || '-'} 剩余=${data.data.remaining_lifespan.toFixed(0)}年`, 'success');
            }
        } catch (e) {
            showToast('评估失败: ' + e.message, 'error');
        }
    });
}

function showToast(message, type = 'info') {
    const container = document.getElementById('toastContainer');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    const icon = { success: '✅', warning: '⚠️', error: '❌', info: 'ℹ️' }[type] || 'ℹ️';
    toast.innerHTML = `<span class="toast-icon">${icon}</span><span>${message}</span>`;
    container.appendChild(toast);
    setTimeout(() => {
        toast.classList.add('closing');
        setTimeout(() => toast.remove(), 300);
    }, 4000);
}

function onWindowResize() {
    const container = document.getElementById('canvasContainer');
    state.camera.aspect = container.clientWidth / container.clientHeight;
    state.camera.updateProjectionMatrix();
    state.renderer.setSize(container.clientWidth, container.clientHeight);
    drawChart();
}

function animate() {
    requestAnimationFrame(animate);
    state.controls.update();

    const t = Date.now() * 0.001;
    state.sensorMarkers.forEach((m, i) => {
        if (m.base !== undefined) {
            const s = 1 + Math.sin(t * 2 + m.phase) * 0.3;
            m.mesh.scale.set(s, s, s);
            m.mesh.material.opacity = 0.4 + Math.sin(t * 2 + m.phase) * 0.2;
        }
    });

    state.renderer.render(state.scene, state.camera);
}

init();
