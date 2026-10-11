import os
from PIL import Image

IMAGES_DIR = '/home/alvo/projects/sec-discovery-suite/docs/images'
OUTPUT_GIF = os.path.join(IMAGES_DIR, 'sds_walkthrough.gif')

# Ordered screenshots
IMAGE_FILES = [
    '01_dashboard_overview.png',
    '02_system_info_modal.png',
    '03_fs_discovery_report.png',
    '04_cert_inspection_drawer.png',
    '05_tls_metadata_report.png',
    '06_cryptographic_relationship_map.png',
    '07_endpoint_flow_details.png',
]

def make_gif():
    print(f"Loading {len(IMAGE_FILES)} images from {IMAGES_DIR}...")
    frames = []
    
    # Target width for optimal README display and reasonable file size
    TARGET_WIDTH = 1200
    
    for filename in IMAGE_FILES:
        filepath = os.path.join(IMAGES_DIR, filename)
        if not os.path.exists(filepath):
            print(f"Warning: {filepath} not found, skipping.")
            continue
        
        img = Image.open(filepath).convert('RGB')
        # Maintain aspect ratio
        w, h = img.size
        target_height = int((TARGET_WIDTH / float(w)) * float(h))
        resized = img.resize((TARGET_WIDTH, target_height), Image.Resampling.LANCZOS)
        
        # Convert to P mode with adaptive palette for crisp rendering
        paletted = resized.convert('P', palette=Image.Palette.ADAPTIVE, colors=256)
        frames.append(paletted)
        print(f"Processed {filename} -> {TARGET_WIDTH}x{target_height}")

    if not frames:
        print("No frames available.")
        return

    print(f"Compiling animated GIF to {OUTPUT_GIF} (2.8s per slide)...")
    # 2800 ms per slide
    frames[0].save(
        OUTPUT_GIF,
        save_all=True,
        append_images=frames[1:],
        duration=2800,
        loop=0,
        optimize=True,
    )

    size_mb = os.path.getsize(OUTPUT_GIF) / (1024 * 1024)
    print(f"Successfully generated {OUTPUT_GIF} ({size_mb:.2f} MB)")

if __name__ == '__main__':
    make_gif()
