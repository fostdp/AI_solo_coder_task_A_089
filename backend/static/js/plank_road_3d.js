import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';

export const rockTextures = {
    '石灰岩': { base: '#6b7a8a', rough: 0.9, detail: [[-10, -5, -8], [8, -12, -3], [-5, 3, -15]] },
    '花岗岩': { base: '#d4d4d4', rough: 0.85, detail: [[-8, -6, -6], [10, -10, -10], [-3, 5, -12]] },
    '片麻岩': { base: '#8a8577', rough: 0.8, detail: [[-12, -4, -7], [6, -14, -5], [-7, 2, -18]] },
    '大理岩': { base: '#f0ebe3', rough: 0.6, detail: [[-6, -8, -9], [9, -6, -11], [-4, 4, -14]] },
    '砂岩':   { base: '#c89b7b', rough: 0.95, detail: [[-15, -3, -5], [12, -8, -8], [-6, 6, -16]] },
    '板岩':   { base: '#5a5a5a', rough: 0.88, detail: [[-9, -7, -6], [7, -13, -9], [-5, 4, -17]] },
};

export const woodColors = {
    '柏木':  { base: '#8B5A2B', grain: '#6B4423', rough: 0.7 },
    '青冈木':{ base: '#A0522D', grain: '#7B3F1A', rough: 0.65 },
    '松木':  { base: '#DEB887', grain: '#B8956A', rough: 0.75 },
    '栎木':  { base: '#B8860B', grain: '#8B6508', rough: 0.6 },
    '杉木':  { base: '#D2B48C', grain: '#B89968', rough: 0.78 },
};

export const gradeColors = {
    'SLIGHT':   '#10b981',
    'MILD':     '#22c55e',
    'MODERATE': '#f59e0b',
    'SERIOUS':  '#f97316',
    'SEVERE':   '#ef4444',
};

let textureParams = null;
export async function loadTextureParams() {
    try {
        const res = await fetch('/texture_params.json');
        textureParams = await res.json();
        console.log('[3D] Texture params loaded:', textureParams);
    } catch (e) {
        console.warn('[3D] Failed to load texture params, using defaults:', e);
    }
}

export function getRockCfg(rockType) {
    if (textureParams?.['岩体纹理PBR参数配置']?.['岩体纹理']?.[rockType]) {
        const tp = textureParams['岩体纹理PBR参数配置']['岩体纹理'][rockType];
        return {
            base: tp['基础颜色_HEX'],
            rough: tp['粗糙度基础'],
            detail: tp['细节']
        };
    }
    return rockTextures[rockType] || rockTextures['石灰岩'];
}

export function getWoodCfg(woodType) {
    if (textureParams?.['岩体纹理PBR参数配置']?.['木材纹理']?.[woodType]) {
        const tp = textureParams['岩体纹理PBR参数配置']['木材纹理'][woodType];
        return {
            base: tp['基础颜色_HEX'],
            grain: tp['年轮颜色_HEX'],
            rough: tp['粗糙度']
        };
    }
    return woodColors[woodType] || woodColors['柏木'];
}

export function initThree(state) {
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

    setupLights(state);
    setupEnvironment(state);
    state.plankRoadGroup = new THREE.Group();
    state.scene.add(state.plankRoadGroup);

    window.addEventListener('resize', () => onWindowResize(state));
    animate(state);
}

function setupLights(state) {
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

function setupEnvironment(state) {
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
        createMountain(state, x, -5 + Math.random() * 3, z, scale);
    }

    for (let i = 0; i < 80; i++) {
        const angle = Math.random() * Math.PI * 2;
        const dist = 40 + Math.random() * 100;
        const x = Math.cos(angle) * dist;
        const z = Math.sin(angle) * dist;
        if (Math.abs(x) < 35 && Math.abs(z) < 25) continue;
        const s = 0.5 + Math.random() * 1.5;
        createTree(state, x, 0, z, s);
    }
}

function createMountain(state, x, y, z, scale) {
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

function createTree(state, x, y, z, scale) {
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

export function buildPlankRoad(state, site) {
    clearGroup(state.plankRoadGroup);
    state.sensorMarkers = [];
    state.crackMeshes = [];

    const rockType = site.RockType || '石灰岩';
    const woodType = site.WoodType || '柏木';
    const rockCfg = getRockCfg(rockType);
    const woodCfg = getWoodCfg(woodType);

    const beamCount = Math.min(site.BeamCount || 50, 80);
    const totalLength = Math.min(site.TotalLength || 50, 100);
    const beamSpacing = totalLength / beamCount;
    const mountainHeight = 15;

    buildRockMountain(state, rockCfg, totalLength, mountainHeight, rockType);
    buildBeamsAndPlanks(state, woodCfg, beamCount, beamSpacing, woodType);
    buildBeamHoles(state, rockCfg, beamCount, beamSpacing, mountainHeight);

    if (state.showCracks) {
        buildCracks(state, rockCfg, totalLength, mountainHeight);
    }

    if (state.showSensors) {
        buildSensorMarkers(state, beamCount, beamSpacing);
    }

    applyStressColors(state);
    updateStatsPanel(state);
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

function buildRockMountain(state, cfg, length, height, rockType) {
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

    state.rockMaterial = createProgressiveRockMaterial(state, cfg, rockType);

    const cliff = new THREE.Mesh(cliffGeo, state.rockMaterial);
    cliff.position.set(0, height / 2 - 2, -10);
    cliff.castShadow = true;
    cliff.receiveShadow = true;
    cliff.userData.needsUpgrade = true;
    cliff.userData.rockType = rockType;
    cliff.userData.cfg = cfg;
    state.plankRoadGroup.add(cliff);

    upgradeRockMaterial(state, cliff, cfg, rockType, 2048);

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

function createProgressiveRockMaterial(state, cfg, rockType) {
    const cacheKey = `${rockType}_low`;
    let lowTex;
    if (state.textureCache[cacheKey]) {
        lowTex = state.textureCache[cacheKey];
    } else {
        const canvas = document.createElement('canvas');
        canvas.width = 512;
        canvas.height = 512;
        const ctx = canvas.getContext('2d');
        generateRockTexture(ctx, cfg, 512);
        lowTex = new THREE.CanvasTexture(canvas);
        lowTex.wrapS = lowTex.wrapT = THREE.RepeatWrapping;
        lowTex.repeat.set(4, 3);
        lowTex.needsUpdate = true;
        state.textureCache[cacheKey] = lowTex;
    }

    return new THREE.MeshStandardMaterial({
        map: lowTex,
        roughness: cfg.rough,
        metalness: 0.05,
        normalScale: new THREE.Vector2(0, 0),
    });
}

function upgradeRockMaterial(state, mesh, cfg, rockType, targetSize) {
    const cacheKey = `${rockType}_${targetSize}`;

    const applyPBR = (maps) => {
        const mat = mesh.material;
        if (!mat) return;
        if (maps.baseColor) {
            if (mat.map) mat.map.dispose();
            mat.map = maps.baseColor;
        }
        if (maps.normalMap) {
            mat.normalMap = maps.normalMap;
            mat.normalScale = new THREE.Vector2(0.8, 0.8);
        }
        if (maps.roughnessMap) {
            mat.roughnessMap = maps.roughnessMap;
        }
        if (maps.aoMap) {
            mat.aoMap = maps.aoMap;
            mat.aoMapIntensity = 0.6;
        }
        mat.needsUpdate = true;
    };

    if (state.textureCache[cacheKey + '_pbr']) {
        applyPBR(state.textureCache[cacheKey + '_pbr']);
        return;
    }

    setTimeout(() => {
        const maps = generatePBRTextures(cfg, targetSize);
        state.textureCache[cacheKey + '_pbr'] = maps;
        applyPBR(maps);
    }, 30);
}

function generatePBRTextures(cfg, size) {
    const baseCanvas = document.createElement('canvas');
    baseCanvas.width = size;
    baseCanvas.height = size;
    const baseCtx = baseCanvas.getContext('2d');
    generateRockTexture(baseCtx, cfg, size);

    const heightData = baseCtx.getImageData(0, 0, size, size).data;

    const normalCanvas = document.createElement('canvas');
    normalCanvas.width = size;
    normalCanvas.height = size;
    const normalCtx = normalCanvas.getContext('2d');
    generateNormalMap(normalCtx, heightData, size, 4.0);

    const roughCanvas = document.createElement('canvas');
    roughCanvas.width = size;
    roughCanvas.height = size;
    const roughCtx = roughCanvas.getContext('2d');
    generateRoughnessMap(roughCtx, heightData, size, cfg.rough);

    const aoCanvas = document.createElement('canvas');
    aoCanvas.width = size;
    aoCanvas.height = size;
    const aoCtx = aoCanvas.getContext('2d');
    generateAOMap(aoCtx, heightData, size);

    const baseColor = new THREE.CanvasTexture(baseCanvas);
    baseColor.wrapS = baseColor.wrapT = THREE.RepeatWrapping;
    baseColor.repeat.set(4, 3);
    baseColor.colorSpace = THREE.SRGBColorSpace;
    baseColor.anisotropy = 8;
    baseColor.needsUpdate = true;

    const normalMap = new THREE.CanvasTexture(normalCanvas);
    normalMap.wrapS = normalMap.wrapT = THREE.RepeatWrapping;
    normalMap.repeat.set(4, 3);
    normalMap.anisotropy = 8;
    normalMap.needsUpdate = true;

    const roughnessMap = new THREE.CanvasTexture(roughCanvas);
    roughnessMap.wrapS = roughnessMap.wrapT = THREE.RepeatWrapping;
    roughnessMap.repeat.set(4, 3);
    roughnessMap.needsUpdate = true;

    const aoMap = new THREE.CanvasTexture(aoCanvas);
    aoMap.wrapS = aoMap.wrapT = THREE.RepeatWrapping;
    aoMap.repeat.set(4, 3);
    aoMap.needsUpdate = true;

    return { baseColor, normalMap, roughnessMap, aoMap };
}

function generateNormalMap(ctx, heightData, size, strength) {
    const imgData = ctx.createImageData(size, size);
    const dst = imgData.data;
    const getPix = (yy, xx) => {
        const ii = (yy * size + xx) * 4;
        return (heightData[ii] + heightData[ii+1] + heightData[ii+2]) / 3;
    };

    for (let y = 0; y < size; y++) {
        for (let x = 0; x < size; x++) {
            const xl = (x - 1 + size) % size;
            const xr = (x + 1) % size;
            const yu = (y - 1 + size) % size;
            const yd = (y + 1) % size;

            const hl = getPix(y, xl);
            const hr = getPix(y, xr);
            const hu = getPix(yu, x);
            const hd = getPix(yd, x);

            let dx = (hl - hr) / 255 * strength;
            let dy = (hu - hd) / 255 * strength;
            const dz = 1.0;

            const len = Math.sqrt(dx*dx + dy*dy + dz*dz);
            dx /= len; dy /= len; const dzN = dz / len;

            const di = (y * size + x) * 4;
            dst[di]     = Math.floor((dx * 0.5 + 0.5) * 255);
            dst[di + 1] = Math.floor((dy * 0.5 + 0.5) * 255);
            dst[di + 2] = Math.floor((dzN * 0.5 + 0.5) * 255);
            dst[di + 3] = 255;
        }
    }
    ctx.putImageData(imgData, 0, 0);
}

function generateRoughnessMap(ctx, heightData, size, baseRough) {
    const imgData = ctx.createImageData(size, size);
    const dst = imgData.data;

    for (let y = 0; y < size; y += 4) {
        for (let x = 0; x < size; x += 4) {
            let sum = 0, minV = 255, maxV = 0;
            for (let dy = 0; dy < 4; dy++) {
                for (let dx = 0; dx < 4; dx++) {
                    const idx = ((y + dy) * size + (x + dx)) * 4;
                    const l = (heightData[idx] + heightData[idx+1] + heightData[idx+2]) / 3;
                    sum += l;
                    if (l < minV) minV = l;
                    if (l > maxV) maxV = l;
                }
            }
            const variance = (maxV - minV) / 255;
            const v = Math.min(1.0, baseRough * 0.7 + variance * 0.8);
            const gray = Math.floor(v * 255);

            for (let dy = 0; dy < 4; dy++) {
                for (let dx = 0; dx < 4; dx++) {
                    const di = ((y + dy) * size + (x + dx)) * 4;
                    dst[di] = gray; dst[di+1] = gray; dst[di+2] = gray; dst[di+3] = 255;
                }
            }
        }
    }
    ctx.putImageData(imgData, 0, 0);
}

function generateAOMap(ctx, heightData, size) {
    const imgData = ctx.createImageData(size, size);
    const dst = imgData.data;
    const tmp = new Float32Array(size * size);

    const block = 8;
    for (let y = 0; y < size; y += block) {
        for (let x = 0; x < size; x += block) {
            let sum = 0;
            for (let dy = 0; dy < block; dy++) {
                for (let dx = 0; dx < block; dx++) {
                    const idx = ((y + dy) * size + (x + dx)) * 4;
                    sum += (heightData[idx] + heightData[idx+1] + heightData[idx+2]) / 3;
                }
            }
            const avg = sum / (block * block);
            for (let dy = 0; dy < block; dy++) {
                for (let dx = 0; dx < block; dx++) {
                    tmp[(y + dy) * size + (x + dx)] = avg;
                }
            }
        }
    }

    const block2 = 32;
    for (let y = 0; y < size; y++) {
        for (let x = 0; x < size; x++) {
            let local = tmp[y * size + x];
            let surround = 0, cnt = 0;
            for (let dy = -block2; dy <= block2; dy += block) {
                for (let dx = -block2; dx <= block2; dx += block) {
                    const yy = Math.min(size-1, Math.max(0, y + dy));
                    const xx = Math.min(size-1, Math.max(0, x + dx));
                    surround += tmp[yy * size + xx];
                    cnt++;
                }
            }
            surround /= cnt;

            let ao = 1.0;
            if (local < surround - 5) {
                ao = 0.5 + (local - (surround - 20)) / 30;
            }
            ao = Math.max(0.3, Math.min(1.0, ao));
            const gray = Math.floor(ao * 255);
            const di = (y * size + x) * 4;
            dst[di] = gray; dst[di+1] = gray; dst[di+2] = gray; dst[di+3] = 255;
        }
    }
    ctx.putImageData(imgData, 0, 0);
}

function generateWoodPBR(cfg, size) {
    const baseCanvas = document.createElement('canvas');
    baseCanvas.width = size;
    baseCanvas.height = size;
    const baseCtx = baseCanvas.getContext('2d');
    generateWoodTexture(baseCtx, cfg, size);

    const heightData = baseCtx.getImageData(0, 0, size, size).data;

    const normalCanvas = document.createElement('canvas');
    normalCanvas.width = size;
    normalCanvas.height = size;
    generateNormalMap(normalCanvas.getContext('2d'), heightData, size, 2.0);

    const roughCanvas = document.createElement('canvas');
    roughCanvas.width = size;
    roughCanvas.height = size;
    generateRoughnessMap(roughCanvas.getContext('2d'), heightData, size, cfg.rough);

    const baseColor = new THREE.CanvasTexture(baseCanvas);
    baseColor.wrapS = baseColor.wrapT = THREE.RepeatWrapping;
    baseColor.repeat.set(1, 2);
    baseColor.colorSpace = THREE.SRGBColorSpace;
    baseColor.needsUpdate = true;

    const normalMap = new THREE.CanvasTexture(normalCanvas);
    normalMap.wrapS = normalMap.wrapT = THREE.RepeatWrapping;
    normalMap.repeat.set(1, 2);
    normalMap.needsUpdate = true;

    const roughnessMap = new THREE.CanvasTexture(roughCanvas);
    roughnessMap.wrapS = roughnessMap.wrapT = THREE.RepeatWrapping;
    roughnessMap.repeat.set(1, 2);
    roughnessMap.needsUpdate = true;

    return { baseColor, normalMap, roughnessMap };
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

function buildBeamsAndPlanks(state, woodCfg, beamCount, beamSpacing, woodType) {
    const lowKey = `${woodType}_wood_low`;
    let woodTex;
    if (state.textureCache[lowKey]) {
        woodTex = state.textureCache[lowKey];
    } else {
        const canvas = document.createElement('canvas');
        canvas.width = 256;
        canvas.height = 256;
        const ctx = canvas.getContext('2d');
        generateWoodTexture(ctx, woodCfg, 256);
        woodTex = new THREE.CanvasTexture(canvas);
        woodTex.wrapS = woodTex.wrapT = THREE.RepeatWrapping;
        woodTex.repeat.set(1, 2);
        woodTex.colorSpace = THREE.SRGBColorSpace;
        woodTex.needsUpdate = true;
        state.textureCache[lowKey] = woodTex;
    }

    state.woodMaterial = new THREE.MeshStandardMaterial({
        map: woodTex,
        roughness: woodCfg.rough,
        metalness: 0.02,
    });

    setTimeout(() => {
        const hiKey = `${woodType}_wood_1024_pbr`;
        let maps;
        if (state.textureCache[hiKey]) {
            maps = state.textureCache[hiKey];
        } else {
            maps = generateWoodPBR(woodCfg, 1024);
            state.textureCache[hiKey] = maps;
        }
        state.woodMaterial.map = maps.baseColor;
        state.woodMaterial.normalMap = maps.normalMap;
        state.woodMaterial.normalScale = new THREE.Vector2(0.4, 0.4);
        state.woodMaterial.roughnessMap = maps.roughnessMap;
        state.woodMaterial.needsUpdate = true;

        state.plankRoadGroup.traverse((obj) => {
            if (obj.isMesh && obj.userData.isWood && obj.material) {
                obj.material.map = maps.baseColor;
                obj.material.normalMap = maps.normalMap;
                obj.material.normalScale = new THREE.Vector2(0.4, 0.4);
                obj.material.roughnessMap = maps.roughnessMap;
                obj.material.needsUpdate = true;
            }
        });
    }, 60);

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

function buildBeamHoles(state, rockCfg, beamCount, beamSpacing, mountainHeight) {
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

function buildCracks(state, rockCfg, length, height) {
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

function buildSensorMarkers(state, beamCount, beamSpacing) {
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

export function applyStressColors(state) {
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

export function updateStatsPanel(state) {
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

function onWindowResize(state) {
    const container = document.getElementById('canvasContainer');
    state.camera.aspect = container.clientWidth / container.clientHeight;
    state.camera.updateProjectionMatrix();
    state.renderer.setSize(container.clientWidth, container.clientHeight);
}

function animate(state) {
    requestAnimationFrame(() => animate(state));

    if (state.controls) {
        state.controls.update();
    }

    const t = performance.now() * 0.001;
    state.sensorMarkers.forEach((item) => {
        if (item.mesh) {
            const scale = 1 + Math.sin(2 * t + item.phase) * 0.3;
            item.mesh.scale.set(scale, scale, 1);
            item.mesh.material.opacity = 0.4 + Math.sin(2 * t + item.phase) * 0.2;
        }
    });

    if (state.renderer && state.scene && state.camera) {
        state.renderer.render(state.scene, state.camera);
    }
}

export function setWireframe(state, enabled) {
    state.wireframe = enabled;
    state.plankRoadGroup.traverse(obj => {
        if (obj.isMesh && obj.material) obj.material.wireframe = enabled;
    });
}
