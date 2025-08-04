// iPhone MTP Explorer - JavaScript

class iPhoneExplorer {
    constructor() {
        this.currentFiles = [];
        this.currentView = 'grid';
        this.init();
    }

    init() {
        this.bindEvents();
        this.loadDevices();
    }

    bindEvents() {
        // Device selection
        document.getElementById('refresh-devices').addEventListener('click', () => {
            this.loadDevices();
        });

        // Help button
        document.getElementById('help-btn').addEventListener('click', () => {
            this.toggleHelp();
        });

        // Browse
        document.getElementById('browse-btn').addEventListener('click', () => {
            this.browse();
        });

        document.getElementById('path-input').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                this.browse();
            }
        });

        // Filters
        document.getElementById('apply-filters').addEventListener('click', () => {
            this.applyFilters();
        });

        document.getElementById('clear-filters').addEventListener('click', () => {
            this.clearFilters();
        });

        // View controls
        document.getElementById('view-grid').addEventListener('click', () => {
            this.setView('grid');
        });

        document.getElementById('view-list').addEventListener('click', () => {
            this.setView('list');
        });

        // Modal
        document.querySelector('.close').addEventListener('click', () => {
            this.closeModal();
        });

        window.addEventListener('click', (e) => {
            if (e.target === document.getElementById('file-modal')) {
                this.closeModal();
            }
        });

        // Auto-fill path when device is selected
        document.getElementById('device-select').addEventListener('change', (e) => {
            if (e.target.value) {
                document.getElementById('path-input').value = e.target.value;
            }
        });
    }

    async loadDevices() {
        try {
            this.showLoading(true);
            const response = await fetch('/api/devices');
            const devices = await response.json();
            
            const select = document.getElementById('device-select');
            select.innerHTML = '<option value="">Sélectionner un périphérique...</option>';
            
            devices.forEach(device => {
                const option = document.createElement('option');
                option.value = device.drive;
                option.textContent = `${device.name} (${device.drive})`;
                select.appendChild(option);
            });

            if (devices.length === 0) {
                this.showMessage('Aucun périphérique MTP détecté. Assurez-vous que votre iPhone est connecté et en mode MTP.', 'warning');
            }

        } catch (error) {
            console.error('Erreur lors du chargement des périphériques:', error);
            this.showMessage('Erreur lors du chargement des périphériques', 'error');
        } finally {
            this.showLoading(false);
        }
    }

    async browse() {
        const path = document.getElementById('path-input').value.trim();
        const recursive = document.getElementById('recursive-check').checked;

        if (!path) {
            this.showMessage('Veuillez entrer un chemin à explorer', 'warning');
            return;
        }

        try {
            this.showLoading(true);
            const response = await fetch(`/api/browse?path=${encodeURIComponent(path)}&recursive=${recursive}`);
            const files = await response.json();

            if (response.ok) {
                this.currentFiles = files;
                this.displayFiles(files);
                this.loadStats();
                this.populateExtensionFilter();
            } else {
                this.showMessage(files.error || 'Erreur lors de l\'exploration', 'error');
            }

        } catch (error) {
            console.error('Erreur lors de l\'exploration:', error);
            this.showMessage('Erreur lors de l\'exploration du répertoire', 'error');
        } finally {
            this.showLoading(false);
        }
    }

    async applyFilters() {
        const category = document.getElementById('category-filter').value;
        const extension = document.getElementById('extension-filter').value;
        const minSizeMB = parseFloat(document.getElementById('min-size').value);
        const maxSizeMB = parseFloat(document.getElementById('max-size').value);

        const params = new URLSearchParams();
        
        if (category && category !== 'all') {
            params.append('category', category);
        }
        if (extension) {
            params.append('extension', extension);
        }
        if (!isNaN(minSizeMB)) {
            params.append('min_size', (minSizeMB * 1024 * 1024).toString());
        }
        if (!isNaN(maxSizeMB)) {
            params.append('max_size', (maxSizeMB * 1024 * 1024).toString());
        }

        try {
            this.showLoading(true);
            const response = await fetch(`/api/filter?${params}`);
            const filteredFiles = await response.json();
            
            this.displayFiles(filteredFiles);
            
        } catch (error) {
            console.error('Erreur lors du filtrage:', error);
            this.showMessage('Erreur lors du filtrage', 'error');
        } finally {
            this.showLoading(false);
        }
    }

    clearFilters() {
        document.getElementById('category-filter').value = 'all';
        document.getElementById('extension-filter').value = '';
        document.getElementById('min-size').value = '';
        document.getElementById('max-size').value = '';
        
        this.displayFiles(this.currentFiles);
    }

    displayFiles(files) {
        const container = document.getElementById('file-container');
        
        if (files.length === 0) {
            container.innerHTML = `
                <div class="no-files">
                    <i class="fas fa-search"></i>
                    <p>Aucun fichier trouvé avec les critères sélectionnés</p>
                </div>
            `;
            return;
        }

        container.innerHTML = '';
        container.className = this.currentView === 'grid' ? 'file-grid' : 'file-list-view';

        files.forEach(file => {
            const fileCard = this.createFileCard(file);
            container.appendChild(fileCard);
        });
    }

    createFileCard(file) {
        const card = document.createElement('div');
        card.className = `file-card ${this.currentView === 'list' ? 'list-item' : ''}`;
        
        const icon = this.getFileIcon(file);
        
        card.innerHTML = `
            <div class="file-icon ${file.category}">
                <i class="${icon}"></i>
            </div>
            <div class="file-name" title="${file.name}">${this.truncateText(file.name, 30)}</div>
            <div class="file-details">
                <div class="file-size">${file.size_formatted}</div>
                <div class="file-date">${file.modified_formatted}</div>
                <div class="file-type">${file.extension.toUpperCase()}</div>
            </div>
        `;

        card.addEventListener('click', () => {
            this.showFileDetails(file);
        });

        return card;
    }

    getFileIcon(file) {
        const iconMap = {
            'image': 'fas fa-image',
            'video': 'fas fa-video',
            'audio': 'fas fa-music',
            'text': 'fas fa-file-alt',
            'document': 'fas fa-file-pdf',
            'archive': 'fas fa-file-archive',
            'other': 'fas fa-file'
        };
        return iconMap[file.category] || iconMap['other'];
    }

    showFileDetails(file) {
        const modal = document.getElementById('file-modal');
        const modalBody = document.getElementById('modal-body');
        
        modalBody.innerHTML = `
            <h2><i class="${this.getFileIcon(file)}"></i> ${file.name}</h2>
            <div class="modal-file-info">
                <div><strong>Chemin:</strong><br>${file.path}</div>
                <div><strong>Taille:</strong><br>${file.size_formatted}</div>
                <div><strong>Type MIME:</strong><br>${file.mime_type}</div>
                <div><strong>Extension:</strong><br>${file.extension}</div>
                <div><strong>Catégorie:</strong><br>${file.category}</div>
                <div><strong>Modifié:</strong><br>${file.modified_formatted}</div>
            </div>
        `;

        modal.style.display = 'block';
    }

    closeModal() {
        document.getElementById('file-modal').style.display = 'none';
    }

    async loadStats() {
        try {
            const response = await fetch('/api/stats');
            const stats = await response.json();
            
            this.displayStats(stats);
            
        } catch (error) {
            console.error('Erreur lors du chargement des statistiques:', error);
        }
    }

    displayStats(stats) {
        const statsPanel = document.getElementById('stats-panel');
        const statsContent = document.getElementById('stats-content');
        
        if (!stats.total_files) {
            statsPanel.style.display = 'none';
            return;
        }

        statsPanel.style.display = 'block';
        
        const categoriesHtml = Object.entries(stats.categories || {})
            .map(([category, count]) => `
                <div class="stat-card">
                    <div class="stat-number">${count}</div>
                    <div class="stat-label">${this.getCategoryLabel(category)}</div>
                </div>
            `).join('');

        statsContent.innerHTML = `
            <div class="stats-grid">
                <div class="stat-card">
                    <div class="stat-number">${stats.total_files}</div>
                    <div class="stat-label">Fichiers total</div>
                </div>
                <div class="stat-card">
                    <div class="stat-number">${stats.total_size_formatted}</div>
                    <div class="stat-label">Taille totale</div>
                </div>
                ${categoriesHtml}
            </div>
        `;
    }

    getCategoryLabel(category) {
        const labels = {
            'image': 'Images',
            'video': 'Vidéos',
            'audio': 'Audio',
            'text': 'Texte',
            'document': 'Documents',
            'archive': 'Archives',
            'other': 'Autres'
        };
        return labels[category] || category;
    }

    populateExtensionFilter() {
        const extensions = [...new Set(this.currentFiles.map(f => f.extension))].filter(Boolean);
        const select = document.getElementById('extension-filter');
        
        // Garder la première option
        const firstOption = select.firstElementChild;
        select.innerHTML = '';
        select.appendChild(firstOption);
        
        extensions.sort().forEach(ext => {
            const option = document.createElement('option');
            option.value = ext;
            option.textContent = ext.toUpperCase();
            select.appendChild(option);
        });
    }

    setView(view) {
        this.currentView = view;
        
        // Update button states
        document.getElementById('view-grid').classList.toggle('active', view === 'grid');
        document.getElementById('view-list').classList.toggle('active', view === 'list');
        
        // Re-render files with new view
        this.displayFiles(this.currentFiles);
    }

    showLoading(show) {
        document.getElementById('loading').style.display = show ? 'block' : 'none';
    }

    showMessage(message, type = 'info') {
        // Créer une notification temporaire
        const notification = document.createElement('div');
        notification.className = `notification notification-${type}`;
        notification.innerHTML = `
            <i class="fas fa-${type === 'error' ? 'exclamation-triangle' : type === 'warning' ? 'exclamation' : 'info-circle'}"></i>
            ${message}
        `;
        
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            padding: 15px 20px;
            background: ${type === 'error' ? '#e74c3c' : type === 'warning' ? '#f39c12' : '#3498db'};
            color: white;
            border-radius: 8px;
            box-shadow: 0 5px 15px rgba(0,0,0,0.2);
            z-index: 1001;
            display: flex;
            align-items: center;
            gap: 10px;
            max-width: 300px;
            word-wrap: break-word;
        `;
        
        document.body.appendChild(notification);
        
        setTimeout(() => {
            notification.remove();
        }, 5000);
    }

    truncateText(text, maxLength) {
        return text.length > maxLength ? text.substring(0, maxLength) + '...' : text;
    }

    toggleHelp() {
        const helpSection = document.getElementById('help-section');
        const isVisible = helpSection.style.display !== 'none';
        helpSection.style.display = isVisible ? 'none' : 'block';
    }
}

// Initialize the explorer when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    new iPhoneExplorer();
});
