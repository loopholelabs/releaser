#!/bin/sh

#	Copyright 2021 Loophole Labs
#
#	Licensed under the Apache License, Version 2.0 (the "License");
#	you may not use this file except in compliance with the License.
#	You may obtain a copy of the License at
#
#		   http://www.apache.org/licenses/LICENSE-2.0
#
#	Unless required by applicable law or agreed to in writing, software
#	distributed under the License is distributed on an "AS IS" BASIS,
#	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#	See the License for the specific language governing permissions and
#	limitations under the License.

set -e

echoerr() {
  printf "$@\n" 1>&2
}

log_info() {
  printf "\033[38;5;61m  ==>\033[0;00m $@\n"
}

log_crit() {
  echoerr
  echoerr "  \033[38;5;125m$@\033[0;00m"
  echoerr
}

is_command() {
  command -v "$1" >/dev/null
}

http_download_curl() {
  local_file=$1
  source_url=$2
  header=$3
  if [ -z "$header" ]; then
    code=$(curl -w '%{http_code}' -fsSL -o "$local_file" "$source_url")
  else
    code=$(curl -w '%{http_code}' -fsSL -H "$header" -o "$local_file" "$source_url")
  fi
  if [ "$code" != "200" ]; then
    log_crit "Error downloading, got $code response from server"
    return 1
  fi
  return 0
}

http_download_wget() {
  local_file=$1
  source_url=$2
  header=$3
  if [ -z "$header" ]; then
    wget -q -O "$local_file" "$source_url"
  else
    wget -q --header "$header" -O "$local_file" "$source_url"
  fi
}

http_download() {
  if is_command curl; then
    http_download_curl "$@"
    return
  elif is_command wget; then
    http_download_wget "$@"
    return
  fi
  log_crit "http_download unable to find wget or curl"
  return 1
}

http_copy() {
  tmp=$(mktemp)
  http_download "${tmp}" "$1" "$2" || return 1
  body=$(cat "$tmp")
  rm -f "${tmp}"
  echo "$body"
}

uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')

  case "$os" in
    msys_nt*) os="windows" ;;
    mingw*) os="windows" ;;
  esac

  # other fixups here
  echo "$os"
}

uname_os_check() {
  os=$(uname_os)
  case "$os" in
    darwin) return 0 ;;
    dragonfly) return 0 ;;
    freebsd) return 0 ;;
    linux) return 0 ;;
    android) return 0 ;;
    nacl) return 0 ;;
    netbsd) return 0 ;;
    openbsd) return 0 ;;
    plan9) return 0 ;;
    solaris) return 0 ;;
    windows) return 0 ;;
  esac
  log_crit "uname_os_check '$(uname -s)' got converted to '$os' which is not a GOOS value."
  return 1
}

uname_arch() {
  arch=$(uname -m)
  case $arch in
    x86_64) arch="amd64" ;;
    x86) arch="386" ;;
    i686) arch="386" ;;
    i386) arch="386" ;;
    aarch64) arch="arm64" ;;
    armv5*) arch="armv5" ;;
    armv6*) arch="armv6" ;;
    armv7*) arch="armv7" ;;
  esac
  echo ${arch}
}

uname_arch_check() {
  arch=$(uname_arch)
  case "$arch" in
    386) return 0 ;;
    amd64) return 0 ;;
    arm64) return 0 ;;
    armv5) return 0 ;;
    armv6) return 0 ;;
    armv7) return 0 ;;
    ppc64) return 0 ;;
    ppc64le) return 0 ;;
    mips) return 0 ;;
    mipsle) return 0 ;;
    mips64) return 0 ;;
    mips64le) return 0 ;;
    s390x) return 0 ;;
    amd64p32) return 0 ;;
  esac
  log_crit "uname_arch_check '$(uname -m)' got converted to '$arch' which is not a GOARCH value."
  return 1
}

mktmpdir() {
  test -z "$TMPDIR" && TMPDIR="$(mktemp -d)"
  mkdir -p "${TMPDIR}"
  echo "${TMPDIR}"
}

start() {
  uname_os_check
  uname_arch_check

  domain="{{domain}}"
  version="{{version}}"
  prefix="{{prefix}}"
  binary="{{binary}}"

  install=${INSTALL:-"/usr/local/bin"}
  tmpDir="$(mktmpdir)"
  tmp="$tmpDir/$binary"

  echo
  log_info "Downloading Version $version for $os $arch"
  http_download $tmp "$prefix://$domain/$version/$os/$arch"

  if [ -w "$install" ]; then
    log_info "Installing $binary to $install"
    tar -xf "$tmp" -O > "$install/$binary"
    chmod +x "$install/$binary"
  else
    otherInstall="$HOME/.config/scale"
    mkdir -p "$otherInstall"
    log_info "Permissions required for installation to $install, using $otherInstall instead — alternatively specify a new directory with:"
    log_info "  $ curl -fsSL $prefix://$domain/$version | INSTALL=. sh"
    tar -xf "$tmp" -O | tee "$otherInstall/$binary" > /dev/null
    chmod +x "$otherInstall/$binary"
    EXPORT_PATH="export PATH=\"\$PATH:$otherInstall\""
    if [ -w "$HOME/.zshrc" ]; then
        if ! grep -q "$EXPORT_PATH" "$HOME/.zshrc" ; then
            log_info "Appending scale source string to ~/.zshrc"
            echo "$EXPORT_PATH" >> "$HOME/.zshrc"
            log_info "Please run 'source ~/.zshrc' to update your current shell or open a new one."
        fi
    elif [ -w "$HOME/.zprofile" ]; then
        if ! grep -q "$EXPORT_PATH" "$HOME/.zprofile" ; then
            log_info "Appending scale source string to ~/.zprofile"
            echo "$EXPORT_PATH" >> "$HOME/.zprofile"
            log_info "Please run 'source ~/.zprofile' to update your current shell or open a new one."
        fi
    elif [ -w "$HOME/.bashrc" ]; then
        if ! grep -q "$EXPORT_PATH" "$HOME/.bashrc" ; then
            log_info "Appending scale source string to ~/.bashrc"
            echo "$EXPORT_PATH" >> "$HOME/.bashrc"
            log_info "Please run 'source ~/.bashrc' to update your current shell or open a new one."
        fi
    elif [ -w "$HOME/.bash_profile" ]; then
        if ! grep -q "$EXPORT_PATH" "$HOME/.bash_profile" ; then
            log_info "Appending scale source string to ~/.bash_profile"
            echo "$EXPORT_PATH" >> "$HOME/.bash_profile"
            log_info "Please run 'source ~/.bash_profile' to update your current shell or open a new one."
        fi
    else
        log_info "Please add the following to your shell profile:"
        log_info "  $ $EXPORT_PATH"
    fi
  fi

  log_info "Installation complete"
  echo
}

start