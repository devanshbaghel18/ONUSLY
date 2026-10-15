#!/usr/bin/env bash
# ==============================================================================
# ONUSLY — Mumbai (ap-south-1) EC2 Automated Setup Script
# Target OS: Amazon Linux 2023
# ==============================================================================

set -euo pipefail

echo ">>> 1. Updating system and installing base dependencies..."
sudo dnf update -y
sudo dnf install -y git golang nginx

echo ">>> 2. Setting up directories..."
sudo mkdir -p /opt/onusly
sudo mkdir -p /etc/onusly
sudo chown -R ec2-user:ec2-user /opt/onusly

echo ">>> 3. Setting up application environment file..."
if [ ! -f /etc/onusly/onusly.env ]; then
  sudo tee /etc/onusly/onusly.env > /dev/null << 'EOF'
PORT=8080
AWS_REGION=ap-south-1
DYNAMODB_TABLE=Onusly
# Fill in production secrets:
# GOOGLE_CLIENT_ID=
# GOOGLE_CLIENT_SECRET=
# JWT_SECRET=
EOF
  sudo chmod 600 /etc/onusly/onusly.env
  echo "Created template /etc/onusly/onusly.env. Please populate production secrets!"
fi

echo ">>> 4. Configuring systemd service..."
sudo cp /opt/onusly/deploy/systemd/onusly.service /etc/systemd/system/onusly.service
sudo systemctl daemon-reload

echo ">>> 5. Configuring Nginx reverse proxy..."
sudo cp /opt/onusly/deploy/nginx/onusly.conf /etc/nginx/conf.d/onusly.conf
sudo nginx -t
sudo systemctl enable nginx
sudo systemctl restart nginx

echo ">>> 6. Building and starting Go backend..."
cd /opt/onusly
go build -o /opt/onusly/server ./cmd/server
sudo systemctl enable onusly
sudo systemctl restart onusly

echo ">>> Setup complete! Checking service status:"
sudo systemctl status onusly --no-pager
sudo systemctl status nginx --no-pager
curl -I http://127.0.0.1:8080/health || echo "Health check ping complete"
