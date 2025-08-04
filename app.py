#!/usr/bin/env python3
"""
iPhone MTP Explorer - Version simplifiée
Scan automatique de l'iPhone au démarrage et affichage web
"""

import os
import json
import mimetypes
import subprocess
import base64
import io
import math
import re
from datetime import datetime
from flask import Flask, render_template, jsonify, request

# Imports pour Windows MTP
try:
    import win32com.client
    import pythoncom
    HAS_WIN32 = True
except ImportError:
    print("❌ Erreur: pywin32 non disponible. Installez-le avec: pip install pywin32")
    HAS_WIN32 = False

app = Flask(__name__)

class SimpleiPhoneScanner:
    def __init__(self):
        self.json_file = "iphone_files_simple.json"
        self.scan_data = {}
        
    def scan_iphone_now(self):
        """Scanner immédiatement l'iPhone et sauvegarder les données"""
        print("🔍 Recherche de l'iPhone...")
        
        if not HAS_WIN32:
            print("❌ Impossible de scanner sans pywin32")
            return False
            
        pythoncom.CoInitialize()
        
        try:
            shell = win32com.client.Dispatch("Shell.Application")
            computer = shell.NameSpace(17)  # My Computer
            
            # Chercher l'iPhone
            iphone_folder = None
            for device in computer.Items():
                if "iPhone" in device.Name or "Apple" in device.Name:
                    iphone_folder = device.GetFolder
                    print(f"📱 iPhone trouvé: {device.Name}")
                    break
                    
            if not iphone_folder:
                print("❌ iPhone non trouvé")
                return False
                
            # Chercher Internal Storage
            storage_folder = None
            for item in iphone_folder.Items():
                if "Internal Storage" in item.Name:
                    storage_folder = item.GetFolder
                    print(f"💾 Stockage trouvé: {item.Name}")
                    break
                    
            if not storage_folder:
                print("❌ Stockage interne non trouvé")
                return False
                
            # Scanner tous les dossiers
            print("📁 Scan des fichiers en cours...")
            all_files = []
            folder_count = 0
            
            for folder_item in storage_folder.Items():
                if folder_item.IsFolder:
                    folder_name = folder_item.Name
                    folder_obj = folder_item.GetFolder
                    folder_count += 1
                    
                    print(f"  📂 Dossier {folder_count}: {folder_name}")
                    
                    # Scanner les fichiers du dossier
                    file_count = 0
                    for file_item in folder_obj.Items():
                        if not file_item.IsFolder:
                            file_count += 1
                            
                            # Récupérer les infos du fichier
                            file_info = self.get_simple_file_info(file_item, folder_obj, folder_name)
                            if file_info:
                                all_files.append(file_info)
                                
                    print(f"    ✅ {file_count} fichiers trouvés")
                    
            # Sauvegarder en JSON
            self.scan_data = {
                "scan_date": datetime.now().isoformat(),
                "total_files": len(all_files),
                "total_folders": folder_count,
                "files": all_files
            }
            
            with open(self.json_file, 'w', encoding='utf-8') as f:
                json.dump(self.scan_data, f, indent=2, ensure_ascii=False)
                
            print(f"🎯 Scan terminé: {len(all_files)} fichiers dans {folder_count} dossiers")
            print(f"💾 Données sauvées dans: {self.json_file}")
            return True
            
        except Exception as e:
            print(f"❌ Erreur scan: {e}")
            return False
        finally:
            pythoncom.CoUninitialize()
            
    def get_simple_file_info(self, file_item, folder_obj, folder_name):
        """Récupérer les infos essentielles d'un fichier"""
        try:
            name = file_item.Name
            
            # Taille du fichier
            size = 0
            try:
                if hasattr(file_item, 'ExtendedProperty'):
                    size = int(file_item.ExtendedProperty("System.Size") or 0)
                elif hasattr(file_item, 'Size') and file_item.Size:
                    size = int(file_item.Size)
            except (AttributeError, ValueError, TypeError):
                size = 1024  # Taille par défaut
                
            # Extension et type
            ext = os.path.splitext(name)[1].lower()
            mime_type, _ = mimetypes.guess_type(name)
            category = self.get_file_type(mime_type, ext)
            
            return {
                "name": name,
                "folder": folder_name,
                "size": size,
                "size_formatted": self.format_size(size),
                "extension": ext,
                "category": category,
                "is_image": category == "image",
                "is_video": category == "video"
            }
            
        except Exception as e:
            print(f"⚠️ Erreur fichier {file_item.Name}: {e}")
            return None
            
    def get_file_type(self, mime_type, ext):
        """Déterminer le type de fichier"""
        if mime_type and mime_type.startswith('image/'):
            return "image"
        elif mime_type and mime_type.startswith('video/'):
            return "video"
        elif ext in ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.webp', '.heic']:
            return "image"
        elif ext in ['.mp4', '.mov', '.avi', '.mkv', '.m4v']:
            return "video"
        elif ext in ['.mp3', '.m4a', '.wav', '.aac']:
            return "audio"
        else:
            return "other"
            
    def format_size(self, size_bytes):
        """Formater la taille"""
        if size_bytes == 0:
            return "0 B"
        
        units = ["B", "KB", "MB", "GB"]
        i = 0
        while size_bytes >= 1024 and i < len(units)-1:
            size_bytes /= 1024
            i += 1
        return f"{size_bytes:.1f} {units[i]}"
        
    def load_scan_data(self):
        """Charger les données du scan"""
        try:
            if os.path.exists(self.json_file):
                with open(self.json_file, 'r', encoding='utf-8') as f:
                    self.scan_data = json.load(f)
                    return True
        except Exception as e:
            print(f"Erreur chargement: {e}")
        return False
        
    def get_files(self, category=None, folders=None, sort_by="name", sort_order="asc"):
        """Récupérer les fichiers avec filtres et tri"""
        if not self.scan_data or "files" not in self.scan_data:
            return []
            
        files = self.scan_data["files"]
        
        # Filtre par catégorie
        if category and category != "all":
            files = [f for f in files if f["category"] == category]
        
        # Filtre par dossiers (liste de dossiers)
        if folders and len(folders) > 0:
            files = [f for f in files if f["folder"] in folders]
            
        # Tri
        if sort_by == "name":
            files.sort(key=lambda x: x["name"].lower(), reverse=(sort_order == "desc"))
        elif sort_by == "size":
            files.sort(key=lambda x: x["size"], reverse=(sort_order == "desc"))
        elif sort_by == "folder":
            files.sort(key=lambda x: x["folder"].lower(), reverse=(sort_order == "desc"))
            
        return files
        
    def get_folders(self):
        """Récupérer la liste des dossiers disponibles"""
        if not self.scan_data or "files" not in self.scan_data:
            return []
            
        folders = set()
        for f in self.scan_data["files"]:
            folders.add(f["folder"])
            
        return sorted(list(folders))
        
    def get_stats(self):
        """Statistiques simples"""
        if not self.scan_data:
            return {}
            
        files = self.scan_data.get("files", [])
        total_size = sum(f["size"] for f in files)
        
        categories = {}
        for f in files:
            cat = f["category"]
            if cat not in categories:
                categories[cat] = 0
            categories[cat] += 1
            
        return {
            "total_files": len(files),
            "total_size": self.format_size(total_size),
            "scan_date": self.scan_data.get("scan_date", ""),
            "categories": categories
        }
    
    def move_files(self, file_names, target_folder, create_folder=False):
        """Déplacer des fichiers vers un autre dossier"""
        if not self.scan_data or "files" not in self.scan_data:
            return {"success": False, "error": "Aucune donnée de scan disponible"}
        
        if not HAS_WIN32:
            return {"success": False, "error": "pywin32 non disponible"}
        
        pythoncom.CoInitialize()
        
        try:
            # Reconnecter à l'iPhone
            shell = win32com.client.Dispatch("Shell.Application")
            computer = shell.NameSpace(17)
            
            # Trouver l'iPhone
            iphone_folder = None
            for device in computer.Items():
                if "iPhone" in device.Name or "Apple" in device.Name:
                    iphone_folder = device.GetFolder
                    break
            
            if not iphone_folder:
                return {"success": False, "error": "iPhone non trouvé"}
            
            # Trouver le stockage interne
            storage_folder = None
            for item in iphone_folder.Items():
                if "Internal Storage" in item.Name:
                    storage_folder = item.GetFolder
                    break
            
            if not storage_folder:
                return {"success": False, "error": "Stockage interne non trouvé"}
            
            # Créer le dossier de destination si nécessaire
            target_folder_obj = None
            if create_folder:
                # Créer un nouveau dossier
                try:
                    # Note: La création de dossier via MTP peut ne pas être supportée
                    # Cette partie peut nécessiter une approche différente
                    print(f"⚠️ Création de dossier '{target_folder}' non implémentée via MTP")
                    return {"success": False, "error": "Création de dossier non supportée"}
                except Exception as e:
                    return {"success": False, "error": f"Impossible de créer le dossier: {e}"}
            else:
                # Trouver le dossier existant
                for folder_item in storage_folder.Items():
                    if folder_item.IsFolder and folder_item.Name == target_folder:
                        target_folder_obj = folder_item.GetFolder
                        break
                
                if not target_folder_obj:
                    return {"success": False, "error": f"Dossier '{target_folder}' non trouvé"}
            
            # Trouver et déplacer les fichiers
            moved_count = 0
            errors = []
            
            for file_name in file_names:
                file_moved = False
                
                # Chercher le fichier dans tous les dossiers
                for folder_item in storage_folder.Items():
                    if not folder_item.IsFolder:
                        continue
                    
                    folder_obj = folder_item.GetFolder
                    for file_item in folder_obj.Items():
                        if not file_item.IsFolder and file_item.Name == file_name:
                            try:
                                # Note: Le déplacement via MTP peut être complexe
                                # Cette implémentation est conceptuelle
                                print(f"⚠️ Déplacement de '{file_name}' non implémenté via MTP")
                                # target_folder_obj.MoveHere(file_item)
                                moved_count += 1
                                file_moved = True
                                break
                            except Exception as e:
                                errors.append(f"Erreur déplacement {file_name}: {e}")
                    
                    if file_moved:
                        break
                
                if not file_moved:
                    errors.append(f"Fichier '{file_name}' non trouvé")
            
            # Mettre à jour les données JSON (simulation)
            if moved_count > 0:
                self.update_file_locations_in_json(file_names, target_folder)
            
            result = {
                "success": moved_count > 0,
                "moved_count": moved_count,
                "errors": errors
            }
            
            if errors:
                result["error"] = f"Quelques erreurs: {'; '.join(errors[:3])}"
            
            return result
            
        except Exception as e:
            return {"success": False, "error": f"Erreur générale: {e}"}
        finally:
            pythoncom.CoUninitialize()
    
    def update_file_locations_in_json(self, file_names, target_folder):
        """Mettre à jour les emplacements des fichiers dans les données JSON"""
        if "files" in self.scan_data:
            for file_data in self.scan_data["files"]:
                if file_data["name"] in file_names:
                    file_data["folder"] = target_folder
            
            # Sauvegarder les modifications
            try:
                with open(self.json_file, 'w', encoding='utf-8') as f:
                    json.dump(self.scan_data, f, indent=2, ensure_ascii=False)
                print(f"📝 Données mises à jour pour {len(file_names)} fichier(s)")
            except Exception as e:
                print(f"⚠️ Erreur sauvegarde JSON: {e}")

# Instance globale
scanner = SimpleiPhoneScanner()

@app.route('/')
def index():
    """Page principale"""
    return render_template('simple.html')

@app.route('/api/files')
def api_files():
    """API pour récupérer les fichiers"""
    category = request.args.get('category', 'all')
    folders = request.args.getlist('folders')  # Récupérer liste de dossiers
    sort_by = request.args.get('sort_by', 'name')
    sort_order = request.args.get('sort_order', 'asc')
    
    files = scanner.get_files(category, folders, sort_by, sort_order)
    return jsonify(files)

@app.route('/api/folders')
def api_folders():
    """API pour récupérer la liste des dossiers"""
    folders = scanner.get_folders()
    return jsonify(folders)

@app.route('/api/stats')
def api_stats():
    """API pour les statistiques"""
    stats = scanner.get_stats()
    return jsonify(stats)

@app.route('/api/rescan')
def api_rescan():
    """API pour relancer le scan"""
    success = scanner.scan_iphone_now()
    return jsonify({"success": success})

@app.route('/api/devices')
def api_devices():
    """API pour obtenir la liste des périphériques"""
    print("⚠️ Fonction de détection de périphériques non implémentée")
    return jsonify([])

@app.route('/api/browse')
def api_browse():
    """API pour explorer un dossier"""
    path = request.args.get('path', '')
    print("⚠️ Fonction de navigation non implémentée")
    return jsonify([])

@app.route('/api/filter')
def api_filter():
    """API pour filtrer les fichiers"""
    category = request.args.get('category')
    folders = request.args.getlist('folders')
    sort_by = request.args.get('sort_by', 'name')
    sort_order = request.args.get('sort_order', 'asc')
    
    filtered_files = scanner.get_files(category, folders, sort_by, sort_order)
    return jsonify(filtered_files)

@app.route('/api/move-files', methods=['POST'])
def api_move_files():
    """API pour déplacer des fichiers"""
    try:
        data = request.get_json()
        file_names = data.get('files', [])
        target_folder = data.get('target_folder', '')
        create_folder = data.get('create_folder', False)
        
        if not file_names:
            return jsonify({"success": False, "error": "Aucun fichier spécifié"})
        
        if not target_folder:
            return jsonify({"success": False, "error": "Dossier de destination non spécifié"})
        
        # Pour la démonstration, on simule le déplacement en mettant à jour seulement le JSON
        # Dans un vrai contexte, il faudrait implémenter le déplacement MTP réel
        scanner.update_file_locations_in_json(file_names, target_folder)
        
        return jsonify({
            "success": True,
            "moved_count": len(file_names),
            "message": f"Fichiers déplacés vers '{target_folder}' (simulation)"
        })
        
    except Exception as e:
        return jsonify({"success": False, "error": f"Erreur serveur: {e}"})

def main():
    """Fonction principale"""
    print("🚀 iPhone Scanner - Version simplifiée")
    print("=" * 50)
    
    # Essayer de charger un scan existant
    if scanner.load_scan_data():
        stats = scanner.get_stats()
        print(f"📄 Données existantes chargées: {stats.get('total_files', 0)} fichiers")
        print(f"📅 Dernier scan: {stats.get('scan_date', 'Inconnu')}")
    else:
        print("📱 Aucune donnée trouvée, scan en cours...")
        if not scanner.scan_iphone_now():
            print("❌ Échec du scan. Vérifiez que l'iPhone est connecté.")
            print("🌐 Serveur web disponible sur: http://localhost:5000")
            print("🔄 Utilisez /api/rescan pour relancer le scan")
    
    print("\n🌐 Lancement du serveur web...")
    print("📱 Interface: http://localhost:5000")
    print("🔄 Pour scanner à nouveau: http://localhost:5000/api/rescan")
    
    app.run(debug=True, host='0.0.0.0', port=5000)

if __name__ == '__main__':
    main()
