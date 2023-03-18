#!/bin/bash

# Replace these variables with your GitHub username, repository name, and asset name
USERNAME="twally3"
REPO_NAME="g"
ASSET_NAME="g-darwin-amd64"

# Get the latest release URL
LATEST_RELEASE=$(curl -s "https://api.github.com/repos/$USERNAME/$REPO_NAME/releases/latest" | grep "browser_download_url.*$ASSET_NAME" | cut -d : -f 2,3 | tr -d \" | xargs)

echo $LATEST_RELEASE

# Download the asset
curl -L -o "g" "${LATEST_RELEASE}"
