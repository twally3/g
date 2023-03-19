#!/usr/bin/env bash

# curl -fsSL https://raw.githubusercontent.com/twally3/g/master/install.sh | bash -s lts

if [ "$(uname)" == "Darwin" ]; then
	OS="darwin"
elif [ "$(expr substr $(uname -s) 1 5)" == "Linux" ]; then
	OS="linux"
# elif [ "$(expr substr $(uname -s) 1 10)" == "MINGW32_NT" ]; then
#     # Do something under 32 bits Windows NT platform
# elif [ "$(expr substr $(uname -s) 1 10)" == "MINGW64_NT" ]; then
#     # Do something under 64 bits Windows NT platform
else
	echo "Unsupported OS $(uname -s)"
	exit 1
fi

if [ "$(uname -m)" == "x86_64" ]; then
	ARCH="amd64"
elif [ "$(uname -m)" == "aarch64" ]; then
	ARCH="arm64"
else
	echo "Unsupported architecture $(uname -m)"
	exit 1
fi

USERNAME="twally3"
REPO_NAME="g"
ASSET_NAME="g-${OS}-${ARCH}"


LATEST_RELEASE=$(curl -s "https://api.github.com/repos/$USERNAME/$REPO_NAME/releases/latest" | grep "browser_download_url.*$ASSET_NAME" | cut -d : -f 2,3 | tr -d \" | xargs)

curl -L -o "g" "${LATEST_RELEASE}"
