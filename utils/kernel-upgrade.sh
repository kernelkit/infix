#!/bin/bash
#
# Automated kernel upgrade test script for LTS 6.12.x
# Run from the infix directory and specify the path to the linux kernel tree
#
# Usage: ./utils/kernel-upgrade.sh <path-to-linux-dir>
#

set -e -o pipefail

# Parse arguments
if [ $# -lt 1 ] || [ $# -gt 2 ]; then
    echo "Usage: $0 <path-to-linux-dir> [branch-name]"
    exit 1
fi

LINUX_DIR="$1"
INFIX_BRANCH="${2:-kernel-upgrade}"

# Configuration
INFIX_DIR="$(dirname "$(dirname "$(readlink -f "$0")")")"
LINUX_BRANCH="kkit-linux-6.12.y"
UPSTREAM_REMOTE="upstream"
UPSTREAM_URL="https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git"
KKIT_REMOTE="origin"

# Detect if running in CI (GitHub Actions sets CI=true)
if [ "${CI:-false}" = "true" ]; then
    KKIT_URL="https://github.com/kernelkit/linux.git"
else
    KKIT_URL="git@github.com:kernelkit/linux.git"
fi

KERNEL_VERSION_PATTERN="6.12"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if directories exist
check_directories() {
    log_info "Checking directories..."

    if [ ! -d "$LINUX_DIR" ]; then
        log_info "Linux directory '$LINUX_DIR' not found, cloning..."
        # Clone to parent directory if LINUX_DIR is a relative path without slashes
        if [[ "$LINUX_DIR" != */* ]]; then
            CLONE_TARGET="../$LINUX_DIR"
            log_info "Cloning to $CLONE_TARGET (parent directory)"
        else
            CLONE_TARGET="$LINUX_DIR"
        fi

        if git clone "$KKIT_URL" "$CLONE_TARGET"; then
            log_info "Successfully cloned linux repository"
            # Update LINUX_DIR to the actual path
            LINUX_DIR="$CLONE_TARGET"
        else
            log_error "Failed to clone linux repository"
            exit 1
        fi
    fi

    if [ ! -d "$INFIX_DIR" ]; then
        log_error "Infix directory '$INFIX_DIR' not found"
        exit 1
    fi

    if [ ! -f "$INFIX_DIR/utils/kernel-refresh.sh" ]; then
        log_error "Kernel refresh script not found at $INFIX_DIR/utils/kernel-refresh.sh"
        exit 1
    fi
}

# Setup remotes for linux kernel tree
setup_linux_remotes() {
    log_info "Setting up linux kernel remotes..."

    # Add or update upstream remote (kernel.org via HTTPS)
    if git -C "$LINUX_DIR" remote get-url "$UPSTREAM_REMOTE" &>/dev/null; then
        log_info "Updating $UPSTREAM_REMOTE remote URL"
        git -C "$LINUX_DIR" remote set-url "$UPSTREAM_REMOTE" "$UPSTREAM_URL"
    else
        log_info "Adding $UPSTREAM_REMOTE remote"
        git -C "$LINUX_DIR" remote add "$UPSTREAM_REMOTE" "$UPSTREAM_URL"
    fi

    # Add or update kkit remote (github via SSH)
    if git -C "$LINUX_DIR" remote get-url "$KKIT_REMOTE" &>/dev/null; then
        log_info "Updating $KKIT_REMOTE remote URL"
        git -C "$LINUX_DIR" remote set-url "$KKIT_REMOTE" "$KKIT_URL"
    else
        log_info "Adding $KKIT_REMOTE remote"
        git -C "$LINUX_DIR" remote add "$KKIT_REMOTE" "$KKIT_URL"
    fi
}

# Update linux kernel tree
update_linux_kernel() {
    log_info "Processing linux kernel tree..."

    # Ensure we're on the correct branch
    log_info "Checking out branch $LINUX_BRANCH"
    git -C "$LINUX_DIR" checkout "$LINUX_BRANCH"

    # Get current version before update
    CURRENT_VERSION=$(git -C "$LINUX_DIR" describe --tags 2>/dev/null || echo "unknown")
    log_info "Current version: $CURRENT_VERSION"

    # Fetch from upstream (kernel.org)
    log_info "Fetching latest kernel updates from upstream..."
    git -C "$LINUX_DIR" fetch "$UPSTREAM_REMOTE"

    # Fetch from kkit remote
    log_info "Fetching from kkit remote..."
    git -C "$LINUX_DIR" fetch "$KKIT_REMOTE"

    # Pull changes from kkit remote
    log_info "Pulling latest changes from $KKIT_REMOTE..."
    git -C "$LINUX_DIR" pull "$KKIT_REMOTE" "$LINUX_BRANCH"
}

# Rebase on new kernel
rebase_kernel() {
    log_info "Rebasing on new kernel release..."

    # Find the latest v6.12.x tag from upstream
    LATEST_TAG=$(git -C "$LINUX_DIR" tag -l "v${KERNEL_VERSION_PATTERN}.*" | sort -V | tail -n1)

    if [ -z "$LATEST_TAG" ]; then
        log_error "No tags found matching v${KERNEL_VERSION_PATTERN}.*"
        exit 1
    fi

    log_info "Latest kernel tag: $LATEST_TAG"
    log_info "Rebasing $LINUX_BRANCH on $LATEST_TAG..."

    if git -C "$LINUX_DIR" rebase "$LATEST_TAG"; then
        log_info "Rebase successful"
    else
        log_error "Rebase failed. Manual intervention required."
        log_info "Run 'git rebase --abort' to cancel or resolve conflicts manually"
        exit 1
    fi
}

# Update infix and run kernel refresh
update_infix() {
    log_info "Processing infix tree..."

    # Fetch latest changes
    log_info "Fetching latest changes..."
    git fetch origin

    # Update main branch
    log_info "Updating main branch..."
    git checkout main
    git pull origin main

    # Check if branch exists and remove it
    if git show-ref --verify --quiet "refs/heads/$INFIX_BRANCH"; then
        log_info "Branch $INFIX_BRANCH exists, removing it..."
        git branch -D "$INFIX_BRANCH"
    fi

    # Create fresh branch from main
    log_info "Creating fresh $INFIX_BRANCH from main..."
    git checkout -b "$INFIX_BRANCH"

    # Get old kernel version from defconfig
    OLD_VERSION=$(grep 'BR2_LINUX_KERNEL_CUSTOM_VERSION_VALUE=' configs/aarch64_defconfig | cut -d'"' -f2)

    if [ -z "$OLD_VERSION" ]; then
        log_error "Could not determine old kernel version from configs/aarch64_defconfig"
        exit 1
    fi

    log_info "Old kernel version: $OLD_VERSION"

    # Get new kernel version from linux tree
    NEW_VERSION=$(cd "$LINUX_DIR" && git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')

    if [ -z "$NEW_VERSION" ]; then
        log_error "Could not determine new kernel version"
        exit 1
    fi

    log_info "New kernel version: $NEW_VERSION"

    # Check if versions are the same
    if [ "$OLD_VERSION" = "$NEW_VERSION" ]; then
        log_info "Kernel version unchanged ($OLD_VERSION), nothing to do"
        exit 0
    fi

    # Run kernel refresh script
    KERNEL_PATH="$(cd "$LINUX_DIR" && pwd)"
    PATCH_DIR="$INFIX_DIR/patches/linux/$NEW_VERSION"
    DEFCONFIG_DIR="$INFIX_DIR/configs"

    log_info "bash utils/kernel-refresh.sh -k \"$KERNEL_PATH\" -o \"$OLD_VERSION\" -t \"v$NEW_VERSION\" -p \"$PATCH_DIR\" -d \"$DEFCONFIG_DIR\""
    log_info "Running kernel refresh script..."
    if bash utils/kernel-refresh.sh -k "$KERNEL_PATH" -o "$OLD_VERSION" -t "v$NEW_VERSION" -p "$PATCH_DIR" -d "$DEFCONFIG_DIR"; then
        log_info "Kernel refresh completed successfully"
    else
        log_error "Kernel refresh failed"
        exit 1
    fi

    # Update changelog
    update_changelog "$NEW_VERSION"

    # Commit all changes
    log_info "Committing changes to infix..."
    git -C "$INFIX_DIR" add -A
    if git -C "$INFIX_DIR" diff-index --quiet HEAD --; then
        log_info "No changes to commit"
    else
        git -C "$INFIX_DIR" commit -m "Upgrade Linux kernel to $NEW_VERSION"
        log_info "Changes committed"
    fi
}

# Update ChangeLog.md with new kernel version
update_changelog() {
    local NEW_VERSION="$1"

    log_info "Updating ChangeLog.md..."
    if [ ! -f "doc/ChangeLog.md" ]; then
        log_warn "doc/ChangeLog.md not found, skipping changelog update"
        return 0
    fi

    # Check if the latest release is UNRELEASED (first release header in file)
    FIRST_RELEASE=$(grep -m1 "^\[v" doc/ChangeLog.md)
    if ! echo "$FIRST_RELEASE" | grep -q "\[UNRELEASED\]"; then
        log_error "First release section in ChangeLog.md is not UNRELEASED"
        log_error "Please create an UNRELEASED section first before running this script"
        exit 1
    fi

    # Extract just the UNRELEASED section (from start until next version header)
    # Need to find the second [v occurrence (first is UNRELEASED itself, second is previous release)
    UNRELEASED_SECTION=$(awk '/^\[v[0-9]/ { count++; if (count == 2) exit } { print }' doc/ChangeLog.md)

    # Check if there's already a kernel upgrade entry in the UNRELEASED section
    if echo "$UNRELEASED_SECTION" | grep -q "^- Upgrade Linux kernel to"; then
        # Update existing kernel upgrade line in UNRELEASED section only
        # Find a real release marker (not UNRELEASED), then replace before that
        awk -v new_version="$NEW_VERSION" '
            /^\[v[0-9]/ && !/UNRELEASED/ { found_version=1 }
            /^- Upgrade Linux kernel to/ && !found_version && !replaced {
                print "- Upgrade Linux kernel to " new_version " (LTS)"
                replaced=1
                next
            }
            {print}
        ' doc/ChangeLog.md > doc/ChangeLog.md.tmp && mv doc/ChangeLog.md.tmp doc/ChangeLog.md
        log_info "Updated existing kernel version entry to $NEW_VERSION"
    else
        # Add new kernel upgrade entry after first "### Changes"
        # Use awk to insert at the right place
        awk -v new_line="- Upgrade Linux kernel to $NEW_VERSION (LTS)" '
            /^### Changes/ && !done {
                print
                getline
                if (NF == 0) print
                print new_line
                done=1
                next
            }
            {print}
        ' doc/ChangeLog.md > doc/ChangeLog.md.tmp && mv doc/ChangeLog.md.tmp doc/ChangeLog.md
        log_info "Added new kernel version entry: $NEW_VERSION"
    fi
}

# Check for uncommitted changes
check_clean_working_tree() {
    log_info "Checking for uncommitted changes..."

    # Check infix directory
    if ! git -C "$INFIX_DIR" diff-index --quiet HEAD --; then
        log_error "Infix directory has uncommitted changes. Please commit or stash them first."
        exit 1
    fi

    # Check linux directory if it exists
    if [ -d "$LINUX_DIR" ]; then
        if ! git -C "$LINUX_DIR" diff-index --quiet HEAD --; then
            log_error "Linux directory has uncommitted changes. Please commit or stash them first."
            exit 1
        fi
    fi

    log_info "Working tree is clean"
}

# Push changes to both repositories
push_changes() {
    # Push rebased linux branch
    log_info "Pushing linux branch to $KKIT_REMOTE..."
    if git -C "$LINUX_DIR" push "$KKIT_REMOTE" "$LINUX_BRANCH" --force-with-lease; then
        log_info "Successfully pushed linux branch to $KKIT_REMOTE"
    else
        log_error "Push to linux remote failed"
        exit 1
    fi

    # Push infix changes
    log_info "Pushing infix branch to origin..."
    if git -C "$INFIX_DIR" push origin "$INFIX_BRANCH"; then
        log_info "Successfully pushed infix branch to origin"
    else
        log_error "Push to infix remote failed"
        exit 1
    fi
}

# Main execution
main() {
    log_info "Starting automated kernel upgrade test..."
    log_info "Working directory: $(pwd)"

    check_clean_working_tree
    check_directories
    setup_linux_remotes
    update_linux_kernel
    rebase_kernel
    update_infix
    push_changes

    log_info "Kernel upgrade completed successfully!"
}

# Run main function
main
