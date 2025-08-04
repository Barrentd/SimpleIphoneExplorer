// Global variables
let allFiles = [];
let allFolders = [];
let selectedFolders = new Set();
let filteredFiles = [];
let currentView = 'grid';

// Initialize the application
document.addEventListener('DOMContentLoaded', function() {
    console.log('iPhone MTP Explorer JSON Mode - Initialisation');
    
    // Event listeners
    setupEventListeners();
    
    // Load folders on startup
    loadFolders();
});

function setupEventListeners() {
    // Scanner controls
    document.getElementById('scan-iphone').addEventListener('click', scanIPhone);
    document.getElementById('load-cache').addEventListener('click', loadCachedData);
    
    // Folder controls
    document.getElementById('refresh-folders').addEventListener('click', loadFolders);
    document.getElementById('select-all-folders').addEventListener('click', selectAllFolders);
    document.getElementById('clear-all-folders').addEventListener('click', clearAllFolders);
    document.getElementById('load-selected').addEventListener('click', loadSelectedFiles);
    
    // Filter controls
    document.getElementById('apply-filters').addEventListener('click', applyFilters);
    document.getElementById('clear-filters').addEventListener('click', clearFilters);
    
    // View controls
    document.getElementById('view-grid').addEventListener('click', () => setView('grid'));
    document.getElementById('view-list').addEventListener('click', () => setView('list'));
    document.getElementById('toggle-stats').addEventListener('click', toggleStats);
    
    // Filter changes
    document.getElementById('category-filter').addEventListener('change', applyFilters);
    document.getElementById('extension-filter').addEventListener('change', applyFilters);
    document.getElementById('sort-by').addEventListener('change', applyFilters);
    document.getElementById('sort-order').addEventListener('change', applyFilters);
}

// Scanner l'iPhone
async function scanIPhone() {
    const scanBtn = document.getElementById('scan-iphone');
    const statusDiv = document.getElementById('scan-status');
    
    scanBtn.disabled = true;
    scanBtn.innerHTML = '<i class="fas fa-spinner fa-spin"></i> Scan en cours...';
    statusDiv.textContent = 'Initialisation du scan...';
    
    try {
        const response = await fetch('/api/scan', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'}
        });
        
        const result = await response.json();
        
        if (result.success) {
            statusDiv.innerHTML = `<span class="success">✓ Scan terminé ! ${result.total_files} fichiers trouvés</span>`;
            // Recharger la liste des dossiers
            await loadFolders();
        } else {
            statusDiv.innerHTML = `<span class="error">✗ Erreur: ${result.error}</span>`;
        }
    } catch (error) {
        console.error('Erreur lors du scan:', error);
        statusDiv.innerHTML = `<span class="error">✗ Erreur de connexion: ${error.message}</span>`;
    } finally {
        scanBtn.disabled = false;
        scanBtn.innerHTML = '<i class="fas fa-sync"></i> Scanner tous les fichiers';
    }
}

// Charger les données du cache
async function loadCachedData() {
    const statusDiv = document.getElementById('scan-status');
    
    try {
        const response = await fetch('/api/selected-files');
        const result = await response.json();
        
        if (result.success) {
            allFiles = result.files;
            filteredFiles = [...allFiles];
            displayFiles();
            updateExtensionFilter();
            updateStats();
            statusDiv.innerHTML = `<span class="success">✓ Cache chargé ! ${allFiles.length} fichiers</span>`;
        } else {
            statusDiv.innerHTML = `<span class="error">✗ Aucun cache disponible</span>`;
        }
    } catch (error) {
        console.error('Erreur lors du chargement du cache:', error);
        statusDiv.innerHTML = `<span class="error">✗ Erreur: ${error.message}</span>`;
    }
}

// Charger la liste des dossiers
async function loadFolders() {
    try {
        const response = await fetch('/api/folders');
        const result = await response.json();
        
        if (result.success) {
            allFolders = result.folders;
            displayFolders();
        } else {
            console.error('Erreur lors du chargement des dossiers:', result.error);
        }
    } catch (error) {
        console.error('Erreur lors du chargement des dossiers:', error);
    }
}

// Afficher les dossiers
function displayFolders() {
    const container = document.getElementById('folders-list');
    
    if (allFolders.length === 0) {
        container.innerHTML = '<div class="no-folders">Aucun dossier disponible. Scannez d\'abord votre iPhone.</div>';
        return;
    }
    
    const foldersHtml = allFolders.map(folder => {
        const isSelected = selectedFolders.has(folder.name);
        return `
            <div class="folder-item ${isSelected ? 'selected' : ''}" data-folder="${folder.name}">
                <div class="folder-checkbox">
                    <input type="checkbox" ${isSelected ? 'checked' : ''} onchange="toggleFolder('${folder.name}')">
                </div>
                <div class="folder-info">
                    <div class="folder-name">${folder.name}</div>
                    <div class="folder-stats">${folder.file_count} fichiers</div>
                </div>
            </div>
        `;
    }).join('');
    
    container.innerHTML = foldersHtml;
}

// Basculer la sélection d'un dossier
async function toggleFolder(folderName) {
    try {
        const response = await fetch(`/api/folders/${encodeURIComponent(folderName)}/toggle`, {
            method: 'POST'
        });
        
        const result = await response.json();
        
        if (result.success) {
            if (result.selected) {
                selectedFolders.add(folderName);
            } else {
                selectedFolders.delete(folderName);
            }
            displayFolders();
        }
    } catch (error) {
        console.error('Erreur lors de la sélection du dossier:', error);
    }
}

// Sélectionner tous les dossiers
function selectAllFolders() {
    allFolders.forEach(folder => {
        selectedFolders.add(folder.name);
        fetch(`/api/folders/${encodeURIComponent(folder.name)}/toggle`, {
            method: 'POST'
        }).catch(console.error);
    });
    displayFolders();
}

// Désélectionner tous les dossiers
function clearAllFolders() {
    selectedFolders.clear();
    allFolders.forEach(folder => {
        fetch(`/api/folders/${encodeURIComponent(folder.name)}/toggle`, {
            method: 'POST'
        }).catch(console.error);
    });
    displayFolders();
}

// Charger les fichiers des dossiers sélectionnés
async function loadSelectedFiles() {
    const statusDiv = document.getElementById('scan-status');
    
    if (selectedFolders.size === 0) {
        statusDiv.innerHTML = '<span class="warning">⚠ Sélectionnez au moins un dossier</span>';
        return;
    }
    
    statusDiv.textContent = 'Chargement des fichiers...';
    
    try {
        const response = await fetch('/api/selected-files');
        const result = await response.json();
        
        if (result.success) {
            allFiles = result.files;
            filteredFiles = [...allFiles];
            displayFiles();
            updateExtensionFilter();
            updateStats();
            statusDiv.innerHTML = `<span class="success">✓ ${allFiles.length} fichiers chargés</span>`;
        } else {
            statusDiv.innerHTML = `<span class="error">✗ Erreur: ${result.error}</span>`;
        }
    } catch (error) {
        console.error('Erreur lors du chargement des fichiers:', error);
        statusDiv.innerHTML = `<span class="error">✗ Erreur: ${error.message}</span>`;
    }
}

// Appliquer les filtres
function applyFilters() {
    const category = document.getElementById('category-filter').value;
    const extension = document.getElementById('extension-filter').value;
    const minSize = parseFloat(document.getElementById('min-size').value) || 0;
    const maxSize = parseFloat(document.getElementById('max-size').value) || Infinity;
    const sortBy = document.getElementById('sort-by').value;
    const sortOrder = document.getElementById('sort-order').value;
    
    // Filtrer
    filteredFiles = allFiles.filter(file => {
        if (category !== 'all' && file.category !== category) return false;
        if (extension && file.extension !== extension) return false;
        
        const sizeInMB = file.size / (1024 * 1024);
        if (sizeInMB < minSize || sizeInMB > maxSize) return false;
        
        return true;
    });
    
    // Trier
    filteredFiles.sort((a, b) => {
        let aVal, bVal;
        
        switch (sortBy) {
            case 'size':
                aVal = a.size;
                bVal = b.size;
                break;
            case 'folder':
                aVal = a.folder_path;
                bVal = b.folder_path;
                break;
            case 'type':
                aVal = a.extension;
                bVal = b.extension;
                break;
            default:
                aVal = a.name.toLowerCase();
                bVal = b.name.toLowerCase();
        }
        
        const comparison = aVal < bVal ? -1 : aVal > bVal ? 1 : 0;
        return sortOrder === 'desc' ? -comparison : comparison;
    });
    
    displayFiles();
    updateStats();
}

// Effacer les filtres
function clearFilters() {
    document.getElementById('category-filter').value = 'all';
    document.getElementById('extension-filter').value = '';
    document.getElementById('min-size').value = '';
    document.getElementById('max-size').value = '';
    document.getElementById('sort-by').value = 'name';
    document.getElementById('sort-order').value = 'asc';
    
    filteredFiles = [...allFiles];
    displayFiles();
    updateStats();
}

// Afficher les fichiers
function displayFiles() {
    const container = document.getElementById('file-container');
    
    if (filteredFiles.length === 0) {
        container.innerHTML = `
            <div class="no-files-message">
                <i class="fas fa-folder-open"></i>
                <p>Aucun fichier trouvé avec ces critères.</p>
            </div>
        `;
        return;
    }
    
    const filesHtml = filteredFiles.map(file => {
        const sizeText = formatFileSize(file.size);
        const iconClass = getFileIcon(file.category, file.extension);
        
        return `
            <div class="file-item">
                <div class="file-icon">
                    <i class="${iconClass}"></i>
                </div>
                <div class="file-info">
                    <div class="file-name" title="${file.name}">${file.name}</div>
                    <div class="file-details">
                        <span class="file-size">${sizeText}</span>
                        <span class="file-type">${file.extension.toUpperCase()}</span>
                    </div>
                    <div class="file-path" title="${file.folder_path}">${file.folder_path}</div>
                </div>
            </div>
        `;
    }).join('');
    
    container.innerHTML = filesHtml;
    container.className = `file-${currentView}`;
}

// Changer la vue
function setView(view) {
    currentView = view;
    
    // Update buttons
    document.getElementById('view-grid').classList.toggle('active', view === 'grid');
    document.getElementById('view-list').classList.toggle('active', view === 'list');
    
    // Update container
    const container = document.getElementById('file-container');
    container.className = `file-${view}`;
}

// Basculer les statistiques
function toggleStats() {
    const panel = document.getElementById('stats-panel');
    panel.style.display = panel.style.display === 'none' ? 'block' : 'none';
    updateStats();
}

// Mettre à jour les statistiques
function updateStats() {
    const panel = document.getElementById('stats-panel');
    if (panel.style.display === 'none') return;
    
    const content = document.getElementById('stats-content');
    
    if (filteredFiles.length === 0) {
        content.innerHTML = '<p>Aucun fichier à analyser</p>';
        return;
    }
    
    // Calculer les statistiques
    const stats = {
        total: filteredFiles.length,
        totalSize: filteredFiles.reduce((sum, file) => sum + file.size, 0),
        categories: {},
        extensions: {},
        folders: {}
    };
    
    filteredFiles.forEach(file => {
        // Par catégorie
        stats.categories[file.category] = (stats.categories[file.category] || 0) + 1;
        
        // Par extension
        stats.extensions[file.extension] = (stats.extensions[file.extension] || 0) + 1;
        
        // Par dossier
        stats.folders[file.folder_path] = (stats.folders[file.folder_path] || 0) + 1;
    });
    
    const statsHtml = `
        <div class="stats-row">
            <div class="stat-item">
                <div class="stat-value">${stats.total}</div>
                <div class="stat-label">Fichiers</div>
            </div>
            <div class="stat-item">
                <div class="stat-value">${formatFileSize(stats.totalSize)}</div>
                <div class="stat-label">Taille totale</div>
            </div>
        </div>
        
        <div class="stats-section">
            <h4>Par catégorie</h4>
            <div class="stats-list">
                ${Object.entries(stats.categories)
                    .sort((a, b) => b[1] - a[1])
                    .map(([cat, count]) => `<div>${cat}: ${count}</div>`)
                    .join('')}
            </div>
        </div>
        
        <div class="stats-section">
            <h4>Extensions les plus fréquentes</h4>
            <div class="stats-list">
                ${Object.entries(stats.extensions)
                    .sort((a, b) => b[1] - a[1])
                    .slice(0, 10)
                    .map(([ext, count]) => `<div>${ext}: ${count}</div>`)
                    .join('')}
            </div>
        </div>
    `;
    
    content.innerHTML = statsHtml;
}

// Mettre à jour le filtre d'extension
function updateExtensionFilter() {
    const select = document.getElementById('extension-filter');
    const extensions = [...new Set(allFiles.map(file => file.extension))].sort();
    
    select.innerHTML = '<option value="">Toutes</option>' +
        extensions.map(ext => `<option value="${ext}">${ext.toUpperCase()}</option>`).join('');
}

// Utilitaires
function formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function getFileIcon(category, extension) {
    const iconMap = {
        'image': 'fas fa-image',
        'video': 'fas fa-video',
        'audio': 'fas fa-music',
        'document': 'fas fa-file-alt',
        'archive': 'fas fa-file-archive',
        'text': 'fas fa-file-text'
    };
    
    return iconMap[category] || 'fas fa-file';
}
