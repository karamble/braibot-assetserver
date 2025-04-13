# Braibot Asset Server

A simple and secure asset server for handling file uploads and downloads. Files are automatically deleted after being downloaded once.

[Braibot](https://github.com/karamble/braibot) is an AI-powered chatbot for [BisonRelay](https://github.com/companyzero/bisonrelay), enabling users to leverage various AI diffusion models via the Fal.ai API for generating images and audio content. Certain API endpoints, such as image-to-image manipulations, audio voice cloning, or audio-to-text conversions, require input files like images or audio. This asset server securely hosts user-provided files and makes them accessible to the fal.ai API for one-time use.


## Features

- Secure file upload with API key authentication
- Random filename generation
- One-time download with automatic file deletion
- Configurable file size limits
- File type restrictions (audio and image files only)
- Nginx configuration included for production use

## Installation

1. Clone the repository:
```bash
git clone https://github.com/karamble/braibot-assetserver.git
cd braibot-assetserver
```

2. Install dependencies:
```bash
go mod tidy
```

3. Configure the server:
Edit `config.json` with your settings:
```json
{
    "max_file_size": 10485760,  // 10MB in bytes
    "api_key": "your-secret-api-key-here",
    "upload_dir": "./uploads",
    "port": ":8080",
    "allowed_types": [
        "image/jpeg",
        "image/png",
        "image/gif",
        "image/webp",
        "image/svg+xml",
        "audio/mpeg",
        "audio/ogg",
        "audio/wav",
        "audio/webm",
        "audio/aac"
    ]
}
```

## Usage

1. Start the server:
```bash
go run main.go
```

2. Upload a file:
```bash
curl -X POST \
  -H "X-API-Key: your-secret-api-key-here" \
  -F "file=@/path/to/your/file.txt" \
  http://localhost:8080/upload
```

3. Download a file:
```bash
curl -O -J http://localhost:8080/download/{random-filename}
```

## File Type Restrictions

The server only accepts the following file types:
- Images: JPEG, PNG, GIF, WebP, SVG
- Audio: MP3, OGG, WAV, WebM, AAC

If you attempt to upload a file with a different content type, the server will reject it with a "File type not allowed" error message.

## Production Setup

1. Build the binary:
```bash
go build -o asset-server
```

2. Use the included nginx.conf as a reverse proxy (modify as needed):
```bash
sudo cp nginx-example.conf /etc/nginx/sites-available/asset-server
sudo ln -s /etc/nginx/sites-available/asset-server /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

## Security Notes

- Change the API key in config.json before deploying
- Use HTTPS in production
- Consider implementing rate limiting
- Regularly update dependencies
- Monitor disk usage 