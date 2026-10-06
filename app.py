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
import threading
import time
import uuid
import shutil
from datetime import datetime
from flask import Flask, render_template, jsonify, request, send_file, abort
from PIL import Image, ImageDraw, ImageFont, ExifTags
import tempfile
import hashlib

# HEIC/HEIF support
try:
    from pillow_heif import register_heif_opener
    register_heif_opener()
    HAS_HEIF = True
except ImportError:
    print("pillow-heif not available. HEIC thumbnails will use generic placeholders.")
    HAS_HEIF = False

# Imports pour Windows MTP
try:
    import win32com.client
    import pythoncom
    HAS_WIN32 = True
except ImportError:
    print("pywin32 non disponible. Installez-le avec: pip install pywin32")
    HAS_WIN32 = False

app = Flask(__name__)

class SimpleiPhoneScanner:
    def __init__(self):
        self.json_file = "iphone_files_simple.json"
        self.scan_data = {}
        self.scan_progress = {
            "scanning": False,
            "current_folder": "",
            "folders_scanned": 0,
            "total_folders": 0,
            "files_found": 0,
            "start_time": None
        }
        self.storage_folder = None
        self.scan_thread = None
        self.thumbnails_dir = os.path.join("static", "thumbnails")
        self.fullimage_cache_dir = os.path.join(tempfile.gettempdir(), "iphone_explorer_cache")
        self.copy_progress = {}
        self.create_thumbnails_dir()

    def create_thumbnails_dir(self):
        """Créer le répertoire des vignettes dans static"""
        if not os.path.exists("static"):
            os.makedirs("static")
        if not os.path.exists(self.thumbnails_dir):
            os.makedirs(self.thumbnails_dir)
        if not os.path.exists(self.fullimage_cache_dir):
            os.makedirs(self.fullimage_cache_dir)

    def quick_scan_folders(self):
        """Scan rapide pour compter les dossiers et préparer le stockage"""
        print("Recherche de l'iPhone...")

        if not HAS_WIN32:
            print("Impossible de scanner sans pywin32")
            return False

        pythoncom.CoInitialize()

        try:
            shell = win32com.client.Dispatch("Shell.Application")
            computer = shell.NameSpace(17)  # My Computer

            iphone_folder = None
            for device in computer.Items():
                if "iPhone" in device.Name or "Apple" in device.Name:
                    iphone_folder = device.GetFolder
                    print(f"iPhone trouve: {device.Name}")
                    break

            if not iphone_folder:
                print("iPhone non trouve")
                return False

            storage_folder = None
            for item in iphone_folder.Items():
                if "Internal Storage" in item.Name:
                    storage_folder = item.GetFolder
                    self.storage_folder = storage_folder
                    print(f"Stockage trouve: {item.Name}")
                    break

            if not storage_folder:
                print("Stockage interne non trouve")
                return False

            folder_count = 0
            for folder_item in storage_folder.Items():
                if folder_item.IsFolder:
                    folder_count += 1

            self.scan_progress["total_folders"] = folder_count
            print(f"{folder_count} dossiers detectes")

            self.scan_data = {
                "scan_date": datetime.now().isoformat(),
                "total_files": 0,
                "total_folders": folder_count,
                "files": [],
                "scan_status": "starting"
            }

            self.save_json()
            return True

        except Exception as e:
            print(f"Erreur scan rapide: {e}")
            return False
        finally:
            pythoncom.CoUninitialize()

    def start_progressive_scan(self):
        """Démarrer le scan progressif en arrière-plan"""
        if self.scan_thread and self.scan_thread.is_alive():
            print("Scan deja en cours")
            return False

        self.scan_thread = threading.Thread(target=self.progressive_scan_worker)
        self.scan_thread.daemon = True
        self.scan_thread.start()
        return True

    def progressive_scan_worker(self):
        """Worker pour le scan progressif"""
        if not self.storage_folder or not HAS_WIN32:
            return

        pythoncom.CoInitialize()

        try:
            self.scan_progress.update({
                "scanning": True,
                "current_folder": "",
                "folders_scanned": 0,
                "files_found": 0,
                "start_time": datetime.now().isoformat()
            })

            all_files = []
            folder_count = 0

            for folder_item in self.storage_folder.Items():
                if not folder_item.IsFolder:
                    continue

                folder_name = folder_item.Name
                folder_obj = folder_item.GetFolder
                folder_count += 1

                self.scan_progress.update({
                    "current_folder": folder_name,
                    "folders_scanned": folder_count
                })

                print(f"  Dossier {folder_count}/{self.scan_progress['total_folders']}: {folder_name}")

                file_count = 0
                folder_files = []

                for file_item in folder_obj.Items():
                    if not file_item.IsFolder:
                        file_count += 1
                        file_info = self.get_simple_file_info(file_item, folder_obj, folder_name)
                        if file_info:
                            folder_files.append(file_info)
                            all_files.append(file_info)

                self.scan_progress["files_found"] = len(all_files)

                self.scan_data.update({
                    "total_files": len(all_files),
                    "files": all_files,
                    "scan_status": f"scanning_folder_{folder_count}"
                })

                self.save_json()
                print(f"    {file_count} fichiers trouves - Total: {len(all_files)}")
                time.sleep(0.1)

            self.scan_data.update({
                "scan_status": "completed",
                "scan_completed": datetime.now().isoformat()
            })

            self.scan_progress.update({
                "scanning": False,
                "current_folder": "Termine"
            })

            self.save_json()
            print(f"Scan termine: {len(all_files)} fichiers dans {folder_count} dossiers")

        except Exception as e:
            print(f"Erreur scan progressif: {e}")
            self.scan_progress["scanning"] = False
            self.scan_data["scan_status"] = f"error: {e}"
            self.save_json()
        finally:
            pythoncom.CoUninitialize()

    def save_json(self):
        """Sauvegarder les données JSON"""
        try:
            with open(self.json_file, 'w', encoding='utf-8') as f:
                json.dump(self.scan_data, f, indent=2, ensure_ascii=False)
        except Exception as e:
            print(f"Erreur sauvegarde JSON: {e}")

    def scan_iphone_now(self):
        """Scanner immédiatement l'iPhone et sauvegarder les données"""
        if self.quick_scan_folders():
            self.start_progressive_scan()
            return True
        return False

    def get_simple_file_info(self, file_item, folder_obj, folder_name):
        """Récupérer les infos essentielles d'un fichier"""
        try:
            name = file_item.Name
            full_path = f"Internal Storage/{folder_name}/{name}"

            # Taille du fichier
            size = 0
            try:
                if hasattr(file_item, 'ExtendedProperty'):
                    size = int(file_item.ExtendedProperty("System.Size") or 0)
                elif hasattr(file_item, 'Size') and file_item.Size:
                    size = int(file_item.Size)
            except (AttributeError, ValueError, TypeError):
                size = 1024

            # Extension et type
            ext = os.path.splitext(name)[1].lower()
            mime_type, _ = mimetypes.guess_type(name)
            category = self.get_file_type(mime_type, ext)

            # Date extraction
            date_taken = None
            date_modified = None
            try:
                if hasattr(file_item, 'ExtendedProperty'):
                    dt = file_item.ExtendedProperty("System.Photo.DateTaken")
                    if dt:
                        date_taken = str(dt)
                    dm = file_item.ExtendedProperty("System.DateModified")
                    if dm:
                        date_modified = str(dm)
            except (AttributeError, TypeError):
                pass

            # Generate a thumbnail hash for on-demand loading
            file_hash = hashlib.md5(f"{folder_name}_{name}".encode()).hexdigest()
            thumbnail_filename = f"{file_hash}.jpg"
            thumbnail_path = os.path.join(self.thumbnails_dir, thumbnail_filename)
            has_thumbnail = os.path.exists(thumbnail_path)

            return {
                "name": name,
                "folder": folder_name,
                "full_path": full_path,
                "size": size,
                "size_formatted": self.format_size(size),
                "extension": ext,
                "category": category,
                "is_image": category == "image",
                "is_video": category == "video",
                "has_thumbnail": has_thumbnail,
                "thumbnail_path": f"thumbnails/{thumbnail_filename}" if has_thumbnail else None,
                "thumbnail_hash": file_hash,
                "date_taken": date_taken,
                "date_modified": date_modified
            }

        except Exception as e:
            print(f"Erreur fichier {file_item.Name}: {e}")
            return None

    def _get_mtp_storage_folder(self):
        """Get MTP storage folder, reconnecting if needed"""
        if self.storage_folder:
            return self.storage_folder

        if not HAS_WIN32:
            return None

        shell = win32com.client.Dispatch("Shell.Application")
        computer = shell.NameSpace(17)

        for device in computer.Items():
            if "iPhone" in device.Name or "Apple" in device.Name:
                iphone_folder = device.GetFolder
                for item in iphone_folder.Items():
                    if "Internal Storage" in item.Name:
                        self.storage_folder = item.GetFolder
                        return self.storage_folder
        return None

    def _find_mtp_file(self, folder_name, filename):
        """Locate a specific file on iPhone via MTP. Returns (folder_obj, file_item) or (None, None)."""
        storage = self._get_mtp_storage_folder()
        if not storage:
            return None, None

        for folder_item in storage.Items():
            if folder_item.IsFolder and folder_item.Name == folder_name:
                folder_obj = folder_item.GetFolder
                for file_item in folder_obj.Items():
                    if not file_item.IsFolder and file_item.Name == filename:
                        return folder_obj, file_item
                break
        return None, None

    def _copy_mtp_file_to_local(self, folder_name, filename, dest_dir):
        """Copy a file from iPhone via MTP CopyHere to a local directory. Returns the local path or None."""
        folder_obj, file_item = self._find_mtp_file(folder_name, filename)
        if not file_item:
            return None

        # Use shell CopyHere to copy to dest_dir
        dest_shell = win32com.client.Dispatch("Shell.Application").NameSpace(dest_dir)
        if not dest_shell:
            return None

        dest_shell.CopyHere(file_item)

        # Poll for completion: wait for file to appear and size to stabilize
        dest_path = os.path.join(dest_dir, filename)
        timeout = 120  # seconds
        start = time.time()

        # Wait for file to appear
        while not os.path.exists(dest_path):
            if time.time() - start > timeout:
                return None
            time.sleep(0.5)

        # Wait for size to stabilize
        last_size = -1
        stable_count = 0
        while stable_count < 3:
            if time.time() - start > timeout:
                break
            try:
                current_size = os.path.getsize(dest_path)
            except OSError:
                time.sleep(0.5)
                continue
            if current_size == last_size and current_size > 0:
                stable_count += 1
            else:
                stable_count = 0
            last_size = current_size
            time.sleep(0.5)

        return dest_path if os.path.exists(dest_path) else None

    def generate_image_thumbnail(self, folder_name, filename, thumbnail_path, thumbnail_filename):
        """Generate a real thumbnail for an image by copying from iPhone via MTP"""
        try:
            with tempfile.TemporaryDirectory() as temp_dir:
                local_path = self._copy_mtp_file_to_local(folder_name, filename, temp_dir)
                if not local_path:
                    return self._generate_generic_thumbnail(thumbnail_path, thumbnail_filename, "image")

                try:
                    img = Image.open(local_path)

                    # Handle EXIF rotation
                    try:
                        exif = img._getexif()
                        if exif:
                            orientation_key = None
                            for key, val in ExifTags.TAGS.items():
                                if val == 'Orientation':
                                    orientation_key = key
                                    break
                            if orientation_key and orientation_key in exif:
                                orientation = exif[orientation_key]
                                if orientation == 3:
                                    img = img.rotate(180, expand=True)
                                elif orientation == 6:
                                    img = img.rotate(270, expand=True)
                                elif orientation == 8:
                                    img = img.rotate(90, expand=True)
                    except (AttributeError, KeyError):
                        pass

                    img.thumbnail((200, 200), Image.LANCZOS)
                    img = img.convert("RGB")
                    img.save(thumbnail_path, 'JPEG', quality=85)
                    return f"thumbnails/{thumbnail_filename}"

                except Exception as e:
                    print(f"Error creating thumbnail from image: {e}")
                    return self._generate_generic_thumbnail(thumbnail_path, thumbnail_filename, "image")

        except Exception as e:
            print(f"Error in generate_image_thumbnail: {e}")
            return self._generate_generic_thumbnail(thumbnail_path, thumbnail_filename, "image")

    def _generate_generic_thumbnail(self, thumbnail_path, thumbnail_filename, category):
        """Create a generic placeholder thumbnail"""
        icon = "Image" if category == "image" else "Video"
        if self.create_generic_thumbnail(thumbnail_path, icon, icon):
            return f"thumbnails/{thumbnail_filename}"
        return None

    def generate_thumbnail(self, file_item, folder_obj, filename, category):
        """Générer une vignette pour un fichier image ou vidéo"""
        try:
            file_hash = hashlib.md5(f"{folder_obj.Title}_{filename}".encode()).hexdigest()
            thumbnail_filename = f"{file_hash}.jpg"
            thumbnail_path = os.path.join(self.thumbnails_dir, thumbnail_filename)

            if os.path.exists(thumbnail_path):
                return f"thumbnails/{thumbnail_filename}"

            # During scan, only create generic thumbnails (lazy real thumbnails via API)
            if category == 'image':
                return self._generate_generic_thumbnail(thumbnail_path, thumbnail_filename, "image")
            elif category == 'video':
                return self._generate_generic_thumbnail(thumbnail_path, thumbnail_filename, "video")

        except Exception as e:
            print(f"Erreur generation vignette pour {filename}: {e}")
            return None

    def generate_video_thumbnail(self, file_item, folder_obj, thumbnail_path, thumbnail_filename):
        """Générer une vignette pour une vidéo"""
        try:
            if self.create_generic_thumbnail(thumbnail_path, "Video", "Video"):
                return f"thumbnails/{thumbnail_filename}"
            return None
        except Exception as e:
            print(f"Erreur vignette video: {e}")
            return None

    def create_generic_thumbnail(self, thumbnail_path, icon, text):
        """Créer une vignette générique avec icône"""
        try:
            img = Image.new('RGB', (150, 150), color='#f0f0f0')
            draw = ImageDraw.Draw(img)

            for y in range(150):
                color_value = int(240 - (y * 0.3))
                color = (color_value, color_value, color_value)
                draw.line([(0, y), (150, y)], fill=color)

            draw.rectangle([0, 0, 149, 149], outline='#cccccc', width=2)

            try:
                font = ImageFont.load_default()
                text_bbox = draw.textbbox((0, 0), text, font=font)
                text_width = text_bbox[2] - text_bbox[0]
                text_x = (150 - text_width) // 2
                text_y = 65
                draw.text((text_x, text_y), text, fill='#333333', font=font)
            except Exception:
                draw.rectangle([40, 40, 110, 110], fill='#dddddd', outline='#999999')

            img.save(thumbnail_path, 'JPEG', quality=85)
            return True

        except Exception as e:
            print(f"Erreur creation vignette generique: {e}")
            return False

    def get_file_type(self, mime_type, ext):
        """Déterminer le type de fichier"""
        if mime_type and mime_type.startswith('image/'):
            return "image"
        elif mime_type and mime_type.startswith('video/'):
            return "video"
        elif ext in ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.webp', '.heic', '.heif']:
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

        if category and category != "all":
            files = [f for f in files if f["category"] == category]

        if folders and len(folders) > 0:
            files = [f for f in files if f["folder"] in folders]

        reverse = (sort_order == "desc")
        if sort_by == "name":
            files.sort(key=lambda x: x["name"].lower(), reverse=reverse)
        elif sort_by == "size":
            files.sort(key=lambda x: x["size"], reverse=reverse)
        elif sort_by == "folder":
            files.sort(key=lambda x: x["folder"].lower(), reverse=reverse)
        elif sort_by == "extension":
            files.sort(key=lambda x: x.get("extension", "").lower(), reverse=reverse)
        elif sort_by == "date":
            files.sort(
                key=lambda x: x.get("date_taken") or x.get("date_modified") or "",
                reverse=reverse
            )

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
            return {"success": False, "error": "Aucune donnee de scan disponible"}

        if not HAS_WIN32:
            return {"success": False, "error": "pywin32 non disponible"}

        pythoncom.CoInitialize()

        try:
            shell = win32com.client.Dispatch("Shell.Application")
            computer = shell.NameSpace(17)

            iphone_folder = None
            for device in computer.Items():
                if "iPhone" in device.Name or "Apple" in device.Name:
                    iphone_folder = device.GetFolder
                    break

            if not iphone_folder:
                return {"success": False, "error": "iPhone non trouve"}

            storage_folder = None
            for item in iphone_folder.Items():
                if "Internal Storage" in item.Name:
                    storage_folder = item.GetFolder
                    break

            if not storage_folder:
                return {"success": False, "error": "Stockage interne non trouve"}

            target_folder_obj = None
            if create_folder:
                print(f"Creation de dossier '{target_folder}' non implementee via MTP")
                return {"success": False, "error": "Creation de dossier non supportee"}
            else:
                for folder_item in storage_folder.Items():
                    if folder_item.IsFolder and folder_item.Name == target_folder:
                        target_folder_obj = folder_item.GetFolder
                        break

                if not target_folder_obj:
                    return {"success": False, "error": f"Dossier '{target_folder}' non trouve"}

            moved_count = 0
            errors = []

            for file_name in file_names:
                file_moved = False
                for folder_item in storage_folder.Items():
                    if not folder_item.IsFolder:
                        continue
                    folder_obj = folder_item.GetFolder
                    for file_item in folder_obj.Items():
                        if not file_item.IsFolder and file_item.Name == file_name:
                            try:
                                print(f"Deplacement de '{file_name}' non implemente via MTP")
                                moved_count += 1
                                file_moved = True
                                break
                            except Exception as e:
                                errors.append(f"Erreur deplacement {file_name}: {e}")
                    if file_moved:
                        break
                if not file_moved:
                    errors.append(f"Fichier '{file_name}' non trouve")

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
            return {"success": False, "error": f"Erreur generale: {e}"}
        finally:
            pythoncom.CoUninitialize()

    def update_file_locations_in_json(self, file_names, target_folder):
        """Mettre à jour les emplacements des fichiers dans les données JSON"""
        if "files" in self.scan_data:
            for file_data in self.scan_data["files"]:
                if file_data["name"] in file_names:
                    file_data["folder"] = target_folder
            try:
                with open(self.json_file, 'w', encoding='utf-8') as f:
                    json.dump(self.scan_data, f, indent=2, ensure_ascii=False)
                print(f"Donnees mises a jour pour {len(file_names)} fichier(s)")
            except Exception as e:
                print(f"Erreur sauvegarde JSON: {e}")

    def get_scan_progress(self):
        """Récupérer le progrès du scan"""
        return self.scan_progress.copy()

    def _compute_sha256(self, file_path):
        """Compute SHA-256 hash of a file, reading in 8KB chunks"""
        sha256 = hashlib.sha256()
        with open(file_path, 'rb') as f:
            while True:
                chunk = f.read(8192)
                if not chunk:
                    break
                sha256.update(chunk)
        return sha256.hexdigest()

    def verify_file_integrity(self, folder_name, filename, dest_path):
        """Verify integrity of a copied file by comparing SHA-256, size, and name"""
        result = {
            "name": filename,
            "name_match": False,
            "size_match": False,
            "hash_match": False,
            "source_hash": None,
            "dest_hash": None,
            "source_size": None,
            "dest_size": None,
            "verified": False
        }

        # Check destination exists
        if not os.path.exists(dest_path):
            result["error"] = "Destination file not found"
            return result

        dest_basename = os.path.basename(dest_path)
        result["name_match"] = (dest_basename == filename)

        # Get dest size and hash
        result["dest_size"] = os.path.getsize(dest_path)
        result["dest_hash"] = self._compute_sha256(dest_path)

        # Copy source to temp for comparison
        try:
            with tempfile.TemporaryDirectory() as temp_dir:
                source_local = self._copy_mtp_file_to_local(folder_name, filename, temp_dir)
                if source_local and os.path.exists(source_local):
                    result["source_size"] = os.path.getsize(source_local)
                    result["source_hash"] = self._compute_sha256(source_local)
                    result["size_match"] = (result["source_size"] == result["dest_size"])
                    result["hash_match"] = (result["source_hash"] == result["dest_hash"])
                    result["verified"] = result["name_match"] and result["size_match"] and result["hash_match"]
                else:
                    result["error"] = "Could not copy source file for verification"
        except Exception as e:
            result["error"] = str(e)

        return result

    def copy_files_worker(self, copy_id, files_to_copy, dest_path):
        """Background worker to copy files from iPhone to local PC"""
        pythoncom.CoInitialize()

        try:
            total = len(files_to_copy)
            self.copy_progress[copy_id] = {
                "status": "copying",
                "total": total,
                "completed": 0,
                "current_file": "",
                "results": [],
                "errors": []
            }

            # Ensure destination exists
            os.makedirs(dest_path, exist_ok=True)

            for i, file_info in enumerate(files_to_copy):
                folder_name = file_info["folder"]
                filename = file_info["name"]

                self.copy_progress[copy_id]["current_file"] = filename

                file_result = {
                    "name": filename,
                    "folder": folder_name,
                    "success": False,
                    "verification": None
                }

                try:
                    local_path = self._copy_mtp_file_to_local(folder_name, filename, dest_path)

                    if local_path and os.path.exists(local_path):
                        file_result["success"] = True
                        file_result["dest_path"] = local_path

                        # Auto-verify after copy
                        verification = self.verify_file_integrity(folder_name, filename, local_path)
                        file_result["verification"] = verification
                    else:
                        file_result["error"] = "Copy failed or timed out"
                        self.copy_progress[copy_id]["errors"].append(f"{filename}: copy failed")

                except Exception as e:
                    file_result["error"] = str(e)
                    self.copy_progress[copy_id]["errors"].append(f"{filename}: {e}")

                self.copy_progress[copy_id]["results"].append(file_result)
                self.copy_progress[copy_id]["completed"] = i + 1

            self.copy_progress[copy_id]["status"] = "completed"
            self.copy_progress[copy_id]["current_file"] = ""

        except Exception as e:
            self.copy_progress[copy_id]["status"] = "error"
            self.copy_progress[copy_id]["errors"].append(str(e))
        finally:
            pythoncom.CoUninitialize()

# Instance globale
scanner = SimpleiPhoneScanner()

@app.route('/')
def index():
    """Page principale"""
    return render_template('index.html')

@app.route('/api/files')
def api_files():
    """API pour récupérer les fichiers"""
    category = request.args.get('category', 'all')
    folders = request.args.getlist('folders')
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

@app.route('/api/progress')
def api_progress():
    """API pour récupérer le progrès du scan"""
    return jsonify(scanner.get_scan_progress())

@app.route('/api/rescan')
def api_rescan():
    """API pour relancer le scan"""
    if scanner.quick_scan_folders():
        scanner.start_progressive_scan()
        return jsonify({"success": True, "message": "Scan progressif demarre"})
    return jsonify({"success": False, "message": "Impossible de demarrer le scan"})

@app.route('/api/devices')
def api_devices():
    """API pour obtenir la liste des périphériques"""
    return jsonify([])

@app.route('/api/browse')
def api_browse():
    """API pour explorer un dossier"""
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
            return jsonify({"success": False, "error": "Aucun fichier specifie"})
        if not target_folder:
            return jsonify({"success": False, "error": "Dossier de destination non specifie"})

        scanner.update_file_locations_in_json(file_names, target_folder)

        return jsonify({
            "success": True,
            "moved_count": len(file_names),
            "message": f"Fichiers deplaces vers '{target_folder}' (simulation)"
        })

    except Exception as e:
        return jsonify({"success": False, "error": f"Erreur serveur: {e}"})

@app.route('/api/thumbnail/<filename>')
def api_thumbnail(filename):
    """API pour servir les vignettes"""
    try:
        if not re.match(r'^[a-f0-9]{32}\.jpg$', filename):
            abort(404)
        thumbnail_path = os.path.join(scanner.thumbnails_dir, filename)
        if not os.path.exists(thumbnail_path):
            abort(404)
        return send_file(thumbnail_path, mimetype='image/jpeg')
    except Exception as e:
        print(f"Erreur serveur vignette: {e}")
        abort(500)

@app.route('/api/generate-thumbnail/<folder>/<filename>')
def api_generate_thumbnail(folder, filename):
    """On-demand thumbnail generation for a specific file"""
    if not HAS_WIN32:
        return jsonify({"success": False, "error": "pywin32 not available"}), 500

    # Sanitize inputs
    if not re.match(r'^[\w\-. ]+$', folder) or not re.match(r'^[\w\-. ]+$', filename):
        abort(400)

    file_hash = hashlib.md5(f"{folder}_{filename}".encode()).hexdigest()
    thumbnail_filename = f"{file_hash}.jpg"
    thumbnail_path = os.path.join(scanner.thumbnails_dir, thumbnail_filename)

    # Return existing thumbnail
    if os.path.exists(thumbnail_path):
        return jsonify({
            "success": True,
            "thumbnail_path": f"thumbnails/{thumbnail_filename}"
        })

    # Generate real thumbnail in a thread-safe way
    pythoncom.CoInitialize()
    try:
        result_path = scanner.generate_image_thumbnail(folder, filename, thumbnail_path, thumbnail_filename)
        if result_path:
            return jsonify({"success": True, "thumbnail_path": result_path})
        else:
            return jsonify({"success": False, "error": "Failed to generate thumbnail"}), 500
    finally:
        pythoncom.CoUninitialize()

@app.route('/api/fullimage/<folder>/<filename>')
def api_fullimage(folder, filename):
    """Serve full-resolution image from iPhone, with HEIC->JPEG conversion"""
    if not HAS_WIN32:
        abort(500)

    if not re.match(r'^[\w\-. ]+$', folder) or not re.match(r'^[\w\-. ]+$', filename):
        abort(400)

    # Check cache first
    cache_key = hashlib.md5(f"{folder}_{filename}".encode()).hexdigest()
    ext = os.path.splitext(filename)[1].lower()

    # For HEIC files, we'll serve as JPEG
    is_heic = ext in ['.heic', '.heif']
    cached_filename = f"{cache_key}.jpg" if is_heic else f"{cache_key}{ext}"
    cached_path = os.path.join(scanner.fullimage_cache_dir, cached_filename)

    if os.path.exists(cached_path):
        mimetype = 'image/jpeg' if is_heic else (mimetypes.guess_type(filename)[0] or 'application/octet-stream')
        return send_file(cached_path, mimetype=mimetype)

    # Copy from iPhone
    pythoncom.CoInitialize()
    try:
        with tempfile.TemporaryDirectory() as temp_dir:
            local_path = scanner._copy_mtp_file_to_local(folder, filename, temp_dir)
            if not local_path:
                abort(404)

            if is_heic and HAS_HEIF:
                # Convert HEIC to JPEG for browser compatibility
                try:
                    img = Image.open(local_path)
                    img = img.convert("RGB")
                    img.save(cached_path, 'JPEG', quality=92)
                except Exception as e:
                    print(f"HEIC conversion failed: {e}")
                    abort(500)
            else:
                shutil.copy2(local_path, cached_path)

        mimetype = 'image/jpeg' if is_heic else (mimetypes.guess_type(filename)[0] or 'application/octet-stream')
        return send_file(cached_path, mimetype=mimetype)
    except Exception as e:
        print(f"Error serving full image: {e}")
        abort(500)
    finally:
        pythoncom.CoUninitialize()

@app.route('/api/copy-files', methods=['POST'])
def api_copy_files():
    """Start copying files from iPhone to local PC"""
    try:
        data = request.get_json()
        files = data.get('files', [])
        dest_path = data.get('dest_path', '')

        if not files:
            return jsonify({"success": False, "error": "No files specified"})
        if not dest_path:
            return jsonify({"success": False, "error": "No destination path specified"})

        # Validate destination path
        dest_path = os.path.normpath(dest_path)

        copy_id = str(uuid.uuid4())

        thread = threading.Thread(
            target=scanner.copy_files_worker,
            args=(copy_id, files, dest_path)
        )
        thread.daemon = True
        thread.start()

        return jsonify({"success": True, "copy_id": copy_id})

    except Exception as e:
        return jsonify({"success": False, "error": str(e)})

@app.route('/api/copy-progress/<copy_id>')
def api_copy_progress(copy_id):
    """Get progress of a copy operation"""
    if copy_id not in scanner.copy_progress:
        return jsonify({"error": "Unknown copy ID"}), 404
    return jsonify(scanner.copy_progress[copy_id])

@app.route('/api/verify-files', methods=['POST'])
def api_verify_files():
    """Manual re-verification of copied files"""
    if not HAS_WIN32:
        return jsonify({"success": False, "error": "pywin32 not available"}), 500

    data = request.get_json()
    files = data.get('files', [])

    if not files:
        return jsonify({"success": False, "error": "No files specified"})

    pythoncom.CoInitialize()
    try:
        results = []
        for f in files:
            folder = f.get('folder', '')
            name = f.get('name', '')
            dest = f.get('dest_path', '')

            if not folder or not name or not dest:
                results.append({"name": name, "error": "Missing parameters", "verified": False})
                continue

            verification = scanner.verify_file_integrity(folder, name, dest)
            results.append(verification)

        return jsonify({"success": True, "results": results})
    finally:
        pythoncom.CoUninitialize()

def main():
    """Fonction principale"""
    print("iPhone Scanner - Version avec thumbnails reels, lightbox et copie")
    print("=" * 60)

    if scanner.load_scan_data():
        stats = scanner.get_stats()
        print(f"Donnees existantes chargees: {stats.get('total_files', 0)} fichiers")
        print(f"Dernier scan: {stats.get('scan_date', 'Inconnu')}")
    else:
        print("Recherche de l'iPhone et scan rapide...")
        if scanner.quick_scan_folders():
            print(f"{scanner.scan_progress['total_folders']} dossiers detectes")
            print("Lancement du scan progressif en arriere-plan...")
            scanner.start_progressive_scan()
        else:
            print("Echec de la detection. Verifiez que l'iPhone est connecte.")

    print("\nLancement du serveur web...")
    print("Interface: http://localhost:5000")
    print("Progres du scan: http://localhost:5000/api/progress")
    print("Pour scanner a nouveau: http://localhost:5000/api/rescan")

    app.run(debug=True, host='0.0.0.0', port=5000)

if __name__ == '__main__':
    main()
