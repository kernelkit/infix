"""
Textual TUI components for support data analysis
"""

from pathlib import Path
from textual.app import App, ComposeResult
from textual.containers import Container, Horizontal, Vertical, VerticalScroll
from textual.widgets import Header, Footer, Static, Label, Tree, DataTable, RichLog, Select, Input
from textual.widgets.tree import TreeNode
from textual.binding import Binding
from textual.screen import Screen
from textual.reactive import reactive
from textual.message import Message
from rich.text import Text
from rich.table import Table as RichTable
from typing import Optional
import re


class FileContentViewer(RichLog):
    """Widget to display file contents with proper text selection support."""

    BINDINGS = [
        Binding("/", "start_search", "Search", show=True),
        Binding("n", "next_match", "Next", show=False),
        Binding("N", "prev_match", "Prev", show=False),
        Binding("ctrl+l", "toggle_line_numbers", "Line #", show=True),
    ]

    class StartSearch(Message):
        """Message sent when search is requested."""
        def __init__(self, viewer):
            super().__init__()
            self.viewer = viewer

    class ToggleLineNumbers(Message):
        """Message sent when line numbers toggle is requested."""
        def __init__(self, viewer):
            super().__init__()
            self.viewer = viewer

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs, wrap=False, highlight=True, markup=False)
        self.border_title = "File Content"
        self.current_file = None
        self.linked_viewer = None  # For scroll synchronization
        self.scroll_locked = False
        self._syncing_scroll = False  # Prevent circular scroll updates
        self._file_lines = []  # Store original lines
        self._search_pattern = None
        self._search_matches = []  # List of line numbers with matches
        self._current_match_index = -1
        self._show_line_numbers = False

    def load_file(self, file_path: Path):
        """Load and display a file."""
        self.current_file = file_path
        self.border_title = f"File: {file_path.name}"

        try:
            content = file_path.read_text()
            self._file_lines = content.splitlines()
            self._refresh_display()
        except Exception as e:
            self.clear()
            self.write(f"Error reading file: {e}", style="bold red")
            self._file_lines = []

    def _refresh_display(self):
        """Refresh the display with current settings (line numbers, search highlights)."""
        self.clear()

        for line_num, line in enumerate(self._file_lines, 1):
            display_line = line

            # Add line numbers if enabled
            if self._show_line_numbers:
                line_prefix = f"{line_num:4d} | "
                display_line = line_prefix + line

            # Highlight search matches
            if self._search_pattern and self._search_pattern.lower() in line.lower():
                # Simple highlight - wrap matches in style tags
                highlighted = self._highlight_matches(display_line, self._search_pattern)
                self.write(Text.from_markup(highlighted))
            else:
                self.write(display_line)

    def _highlight_matches(self, text: str, pattern: str) -> str:
        """Highlight search pattern in text (case insensitive)."""
        if not pattern:
            return text

        # Use regex for case-insensitive replacement
        def replacer(match):
            return f"[black on yellow]{match.group(0)}[/black on yellow]"

        try:
            highlighted = re.sub(re.escape(pattern), replacer, text, flags=re.IGNORECASE)
            return highlighted
        except:
            return text

    def search(self, pattern: str):
        """Search for pattern in file and highlight matches."""
        self._search_pattern = pattern
        self._search_matches = []
        self._current_match_index = -1

        if pattern:
            # Find all lines with matches
            for line_num, line in enumerate(self._file_lines, 1):
                if pattern.lower() in line.lower():
                    self._search_matches.append(line_num)

            if self._search_matches:
                self._current_match_index = 0
                # Update title with match count
                self.border_title = f"File: {self.current_file.name} - {len(self._search_matches)} matches"
                # Jump to first match
                self._jump_to_current_match()
            else:
                self.border_title = f"File: {self.current_file.name} - No matches"

        self._refresh_display()

    def _jump_to_current_match(self):
        """Scroll to the current match."""
        if self._current_match_index >= 0 and self._current_match_index < len(self._search_matches):
            line_num = self._search_matches[self._current_match_index]
            # Scroll to make the line visible (approximate)
            # RichLog doesn't have direct line-based scrolling, so we estimate
            total_lines = len(self._file_lines)
            if total_lines > 0:
                scroll_ratio = (line_num - 1) / total_lines
                # This is approximate - adjust max scroll as needed
                self.scroll_y = scroll_ratio * self.max_scroll_y if self.max_scroll_y > 0 else 0

    def action_next_match(self):
        """Jump to next search match."""
        if self._search_matches:
            self._current_match_index = (self._current_match_index + 1) % len(self._search_matches)
            self._jump_to_current_match()
            # Update title to show current match position
            current = self._current_match_index + 1
            total = len(self._search_matches)
            self.border_title = f"File: {self.current_file.name} - Match {current}/{total}"

    def action_prev_match(self):
        """Jump to previous search match."""
        if self._search_matches:
            self._current_match_index = (self._current_match_index - 1) % len(self._search_matches)
            self._jump_to_current_match()
            # Update title to show current match position
            current = self._current_match_index + 1
            total = len(self._search_matches)
            self.border_title = f"File: {self.current_file.name} - Match {current}/{total}"

    def action_toggle_line_numbers(self):
        """Toggle line numbers display - post message for parent to handle."""
        # Post message to parent so it can coordinate between panes
        self.post_message(self.ToggleLineNumbers(self))

    def toggle_line_numbers_internal(self):
        """Actually toggle line numbers (called by parent)."""
        self._show_line_numbers = not self._show_line_numbers
        self._refresh_display()
        return self._show_line_numbers

    def action_start_search(self):
        """Start search - post a message that parent can handle."""
        # Post a message to notify parent
        self.post_message(self.StartSearch(self))

    def clear_content(self):
        """Clear the viewer."""
        self.clear()
        self.border_title = "File Content"
        self.current_file = None
        self._file_lines = []
        self._search_pattern = None
        self._search_matches = []

    def on_mount(self) -> None:
        """Set up scroll watching."""
        self.watch(self, "scroll_y", self._on_scroll_change)

    def _on_scroll_change(self, old_y: float, new_y: float) -> None:
        """Handle scroll position changes."""
        if self._syncing_scroll or not self.scroll_locked or not self.linked_viewer:
            return

        # Sync scroll to linked viewer
        if self.linked_viewer and new_y != old_y:
            self.linked_viewer._syncing_scroll = True
            self.linked_viewer.scroll_y = new_y
            self.linked_viewer._syncing_scroll = False


class SupportTree(Tree):
    """Custom tree widget for navigating support archive structure."""

    def __init__(self, archive, *args, **kwargs):
        super().__init__("Support Data", *args, **kwargs)
        self.archive = archive
        self.show_root = True
        self.guide_depth = 4

        # Build tree structure
        self._build_tree()

    def _build_tree(self):
        """Build the tree structure from archive."""
        structure = self.archive.get_structure()

        # Add categories as top-level nodes
        for category, files in sorted(structure.items()):
            if not files:
                continue

            category_node = self.root.add(f"📁 {category}", expand=True)
            category_node.data = {"type": "category", "name": category}

            # Group files by subdirectory - keep full paths
            file_tree = {}
            for file_path in sorted(files):
                parts = list(file_path.parts)
                if parts[0] == category:
                    parts = parts[1:]  # Remove category prefix

                # Build nested structure, storing full Path objects
                current = file_tree
                for i, part in enumerate(parts):
                    if i == len(parts) - 1:
                        # It's a file - store the full path
                        if part not in current:
                            current[part] = file_path  # Store full relative path
                    else:
                        # It's a directory
                        if part not in current:
                            current[part] = {}
                        elif not isinstance(current[part], dict):
                            # Edge case: name collision
                            current[part] = {}
                        current = current[part]

            # Add to tree
            self._add_tree_nodes(category_node, file_tree)

    def _add_tree_nodes(self, parent_node: TreeNode, tree_dict: dict):
        """Recursively add nodes to the tree."""
        for name, subtree in sorted(tree_dict.items()):
            if isinstance(subtree, Path):
                # It's a file - use the stored full path
                full_path = self.archive.path / subtree
                node = parent_node.add_leaf(f"📄 {name}")
                node.data = {"type": "file", "path": full_path}
            elif isinstance(subtree, dict):
                # It's a directory
                dir_node = parent_node.add(f"📁 {name}", expand=False)
                dir_node.data = {"type": "directory", "name": name}
                self._add_tree_nodes(dir_node, subtree)


class SummaryView(Container):
    """View displaying summary of all archives with interactive selection."""

    BINDINGS = [
        Binding("space", "toggle_selection", "Select/Deselect", show=True),
        Binding("enter", "confirm_selection", "Confirm", show=True),
    ]

    def __init__(self, archives, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.archives = archives
        self.selected_indices = set()  # Track selected archive indices
        self.border_title = "Summary - Select archive(s) to analyze"

    def compose(self) -> ComposeResult:
        """Create the summary layout."""
        # Instructions
        yield Static("Use Space to select/deselect archives, Enter to confirm (1 for analyze, 2 for compare)",
                    classes="summary-instructions")
        # Interactive table
        table = DataTable(id="archive-table", cursor_type="row")
        table.zebra_stripes = True
        yield table

    def on_mount(self) -> None:
        """Populate the table when mounted."""
        table = self.query_one("#archive-table", DataTable)

        # Add columns
        table.add_column("✓", key="selected", width=3)  # Selection indicator
        table.add_column("Hostname", key="hostname")
        table.add_column("Timestamp", key="timestamp")
        table.add_column("Uptime", key="uptime")
        table.add_column("Load", key="load")
        table.add_column("Memory", key="memory")
        table.add_column("Issues", key="issues")

        # Add rows
        for idx, archive in enumerate(self.archives):
            data = archive.get_summary_data()

            # Format data
            uptime_str = self._format_uptime(data["uptime_seconds"])
            mem_pct = data["memory_percent"]
            mem_str = f"{data['memory_used_mb']}M/{data['memory_total_mb']}M ({mem_pct}%)"

            # Format issues
            errors = data["dmesg_errors"]
            warnings = data["dmesg_warnings"]
            if errors > 0:
                issues_str = f"{errors} err"
                if warnings > 0:
                    issues_str += f", {warnings} warn"
            elif warnings > 0:
                issues_str = f"{warnings} warn"
            else:
                issues_str = "none"

            table.add_row(
                "",  # Selection indicator (empty initially)
                archive.hostname,
                archive.timestamp or "unknown",
                uptime_str,
                data["load_avg"],
                mem_str,
                issues_str,
                key=str(idx)
            )

        # Focus the table
        table.focus()

    def action_toggle_selection(self) -> None:
        """Toggle selection of current row."""
        table = self.query_one("#archive-table", DataTable)
        if table.cursor_row is None:
            return

        # Get the cursor position (row index)
        cursor_row_idx = table.cursor_row

        # Get all row keys and find the one at cursor position
        row_keys = list(table.rows.keys())
        if cursor_row_idx >= len(row_keys):
            return

        row_key = row_keys[cursor_row_idx]
        idx = int(row_key.value)

        # Toggle selection
        if idx in self.selected_indices:
            self.selected_indices.remove(idx)
            # Update the row to remove checkmark
            table.update_cell(row_key, "selected", "")
        else:
            self.selected_indices.add(idx)
            # Update the row to add checkmark
            table.update_cell(row_key, "selected", "✓")

        # Update border title with selection count
        count = len(self.selected_indices)
        if count == 0:
            self.border_title = "Summary - Select archive(s) to analyze"
        elif count == 1:
            self.border_title = f"Summary - {count} archive selected (analyze)"
        elif count == 2:
            self.border_title = f"Summary - {count} archives selected (compare)"
        else:
            self.border_title = f"Summary - {count} archives selected (too many for compare)"

    def on_data_table_row_selected(self, event: DataTable.RowSelected) -> None:
        """Handle Enter key on a row - same as confirm selection."""
        self.action_confirm_selection()

    def action_confirm_selection(self) -> None:
        """Confirm selection and switch to appropriate view."""
        if len(self.selected_indices) == 0:
            self.app.notify("No archives selected. Use Space to select.", severity="warning", timeout=2)
            return
        elif len(self.selected_indices) == 1:
            # Single archive - analyze mode
            idx = list(self.selected_indices)[0]
            self.app.selected_archives = [self.archives[idx]]
            self.app.current_mode = "analyze"
        elif len(self.selected_indices) == 2:
            # Two archives - compare mode
            indices = sorted(self.selected_indices)
            self.app.selected_archives = [self.archives[indices[0]], self.archives[indices[1]]]
            # Debug: show which archives are selected
            arch1 = self.archives[indices[0]].hostname
            arch2 = self.archives[indices[1]].hostname
            self.app.notify(f"Comparing: {arch1} vs {arch2}", timeout=2)
            self.app.current_mode = "compare"
        else:
            self.app.notify(f"Selected {len(self.selected_indices)} archives. Select 1 for analyze or 2 for compare.",
                          severity="warning", timeout=3)

    def _format_uptime(self, seconds: int) -> str:
        """Format uptime seconds into human-readable string."""
        if seconds < 0:  # Only negative indicates truly unknown
            return "unknown"

        days = seconds // 86400
        hours = (seconds % 86400) // 3600
        minutes = (seconds % 3600) // 60

        if days > 0:
            return f"{days}d {hours}h"
        elif hours > 0:
            return f"{hours}h {minutes}m"
        else:
            return f"{minutes}m"


class ComparisonPane(Container):
    """Single pane in comparison view with file selector and viewer."""

    def __init__(self, archive, pane_id: str, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.archive = archive
        self.pane_id = pane_id
        self.border_title = f"{archive.hostname} @ {archive.timestamp or 'unknown'}"
        self.current_file = None
        self._search_visible = False

    def compose(self) -> ComposeResult:
        """Create the pane layout."""
        # Search input (initially hidden)
        search_input = Input(placeholder="Search (press Enter)", id=f"search-input-{self.pane_id}")
        search_input.display = False
        yield search_input

        # File selector dropdown
        yield Static("Select file:", classes="file-selector-label")
        yield Select(
            options=self._get_file_options(),
            prompt="Choose a file...",
            id=f"file-select-{self.pane_id}",
            classes="file-selector"
        )
        # File viewer
        viewer = FileContentViewer(id=f"viewer-{self.pane_id}")
        viewer.border_title = "File Content"
        yield viewer

    def _get_file_options(self):
        """Get list of all files in archive as select options."""
        options = []
        structure = self.archive.get_structure()

        for category in sorted(structure.keys()):
            files = structure[category]
            for file_path in sorted(files):
                # Create a readable label with category prefix
                label = str(file_path)
                value = str(file_path)
                options.append((label, value))

        return options

    def on_select_changed(self, event: Select.Changed) -> None:
        """Handle file selection change."""
        if event.select.id == f"file-select-{self.pane_id}" and event.value != Select.BLANK:
            file_path = Path(event.value)
            full_path = self.archive.path / file_path
            viewer = self.query_one(f"#viewer-{self.pane_id}", FileContentViewer)
            viewer.load_file(full_path)
            self.current_file = file_path

    def on_file_content_viewer_start_search(self, message: FileContentViewer.StartSearch) -> None:
        """Handle search request from viewer."""
        self.action_start_search()

    def load_file(self, relative_path: Path) -> bool:
        """Load a specific file by relative path. Returns True if successful."""
        full_path = self.archive.path / relative_path
        if full_path.exists():
            viewer = self.query_one(f"#viewer-{self.pane_id}", FileContentViewer)
            viewer.load_file(full_path)
            self.current_file = relative_path
            # Update selector
            selector = self.query_one(f"#file-select-{self.pane_id}", Select)
            selector.value = str(relative_path)
            return True
        return False

    def action_start_search(self) -> None:
        """Show search input for this pane."""
        search_input = self.query_one(f"#search-input-{self.pane_id}", Input)
        search_input.display = True
        self._search_visible = True
        search_input.focus()

    def on_input_submitted(self, event: Input.Submitted) -> None:
        """Handle search submission."""
        if event.input.id == f"search-input-{self.pane_id}":
            pattern = event.value
            viewer = self.query_one(f"#viewer-{self.pane_id}", FileContentViewer)
            viewer.search(pattern)

            # Hide search input and refocus viewer
            search_input = self.query_one(f"#search-input-{self.pane_id}", Input)
            search_input.display = False
            self._search_visible = False
            viewer.focus()

            # Notify parent to sync search to other pane
            self.post_message(self.SearchSubmitted(self.pane_id, pattern))

    class SearchSubmitted(Message):
        """Message sent when search is submitted in a pane."""
        def __init__(self, pane_id: str, pattern: str):
            super().__init__()
            self.pane_id = pane_id
            self.pattern = pattern

    def on_key(self, event) -> None:
        """Handle escape key to close search input."""
        if event.key == "escape" and self._search_visible:
            search_input = self.query_one(f"#search-input-{self.pane_id}", Input)
            search_input.display = False
            self._search_visible = False
            viewer = self.query_one(f"#viewer-{self.pane_id}", FileContentViewer)
            viewer.focus()
            event.stop()
            event.prevent_default()


class ComparisonView(Container):
    """View for comparing two archives side-by-side."""

    scroll_locked = reactive(False)

    def __init__(self, archive1, archive2, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.archive1 = archive1
        self.archive2 = archive2
        self._syncing = False  # Prevent circular updates

    def compose(self) -> ComposeResult:
        """Create the comparison layout."""
        with Horizontal():
            # Left pane
            yield ComparisonPane(self.archive1, "left", id="pane-left", classes="comparison-pane")
            # Right pane
            yield ComparisonPane(self.archive2, "right", id="pane-right", classes="comparison-pane")

    def on_mount(self) -> None:
        """Try to load operational-config.json or a common file."""
        # Preference: operational-config.json
        preferred = Path("operational-config.json")

        left_pane = self.query_one("#pane-left", ComparisonPane)
        right_pane = self.query_one("#pane-right", ComparisonPane)

        # Try preferred file first
        left_loaded = left_pane.load_file(preferred)
        right_loaded = right_pane.load_file(preferred)

        # If preferred didn't work, find any common file
        if not (left_loaded and right_loaded):
            structure1 = self.archive1.get_structure()
            structure2 = self.archive2.get_structure()

            # Flatten file lists
            files1 = set()
            for files in structure1.values():
                files1.update(str(f) for f in files)

            files2 = set()
            for files in structure2.values():
                files2.update(str(f) for f in files)

            # Find common files
            common = files1 & files2

            if common:
                # Load the first common file (sorted for consistency)
                first_common = Path(sorted(common)[0])
                left_pane.load_file(first_common)
                right_pane.load_file(first_common)

        # Set up scroll event watchers
        self._setup_scroll_sync()

        # Notify that scroll is locked by default
        self.app.notify("Scroll locked (press 'l' to unlock)", timeout=2)

        # Set focus to left viewer so keys work immediately
        try:
            left_viewer = self.query_one("#viewer-left", FileContentViewer)
            left_viewer.focus()
        except:
            pass

    def _setup_scroll_sync(self) -> None:
        """Set up scroll synchronization between panes."""
        try:
            left_viewer = self.query_one("#viewer-left", FileContentViewer)
            right_viewer = self.query_one("#viewer-right", FileContentViewer)

            # Link viewers for scroll sync
            left_viewer.linked_viewer = right_viewer
            right_viewer.linked_viewer = left_viewer

            # Enable scroll lock by default in compare mode
            self.scroll_locked = True
            left_viewer.scroll_locked = True
            right_viewer.scroll_locked = True

            # Store references
            self._left_viewer = left_viewer
            self._right_viewer = right_viewer
        except:
            pass

    def on_select_changed(self, event: Select.Changed) -> None:
        """Handle file selection - sync to other pane if same file exists."""
        if self._syncing:
            return

        if event.select.id == "file-select-left" and event.value != Select.BLANK:
            # Left pane changed, try to load same file in right
            file_path = Path(event.value)
            right_pane = self.query_one("#pane-right", ComparisonPane)
            self._syncing = True
            right_pane.load_file(file_path)  # Will fail gracefully if doesn't exist
            self._syncing = False

        elif event.select.id == "file-select-right" and event.value != Select.BLANK:
            # Right pane changed, try to load same file in left
            file_path = Path(event.value)
            left_pane = self.query_one("#pane-left", ComparisonPane)
            self._syncing = True
            left_pane.load_file(file_path)  # Will fail gracefully if doesn't exist
            self._syncing = False

    def action_toggle_lock(self) -> None:
        """Toggle scroll lock between panes."""
        self.scroll_locked = not self.scroll_locked

        # Propagate lock state to viewers
        if hasattr(self, '_left_viewer') and hasattr(self, '_right_viewer'):
            self._left_viewer.scroll_locked = self.scroll_locked
            self._right_viewer.scroll_locked = self.scroll_locked

            # Sync current scroll positions when locking
            if self.scroll_locked:
                # Sync right to left's position
                self._right_viewer._syncing_scroll = True
                self._right_viewer.scroll_y = self._left_viewer.scroll_y
                self._right_viewer._syncing_scroll = False

        status = "locked" if self.scroll_locked else "unlocked"
        self.app.notify(f"Scroll {status}", timeout=1)

    def on_file_content_viewer_toggle_line_numbers(self, message: FileContentViewer.ToggleLineNumbers) -> None:
        """Handle line numbers toggle from either viewer - sync both."""
        if hasattr(self, '_left_viewer') and hasattr(self, '_right_viewer'):
            # Toggle both viewers
            new_state = self._left_viewer.toggle_line_numbers_internal()
            self._right_viewer._show_line_numbers = new_state
            self._right_viewer._refresh_display()

            status = "on" if new_state else "off"
            self.app.notify(f"Line numbers {status}", timeout=1)

    def on_comparison_pane_search_submitted(self, message) -> None:
        """Handle search submitted in one pane - sync to other pane."""
        # Apply the search to the other pane
        if message.pane_id == "left":
            # Search was in left, apply to right
            right_viewer = self.query_one("#viewer-right", FileContentViewer)
            right_viewer.search(message.pattern)
        else:
            # Search was in right, apply to left
            left_viewer = self.query_one("#viewer-left", FileContentViewer)
            left_viewer.search(message.pattern)

    def action_start_search(self) -> None:
        """Start search in the focused pane."""
        # Find which pane has focus and trigger search there
        try:
            left_pane = self.query_one("#pane-left", ComparisonPane)
            right_pane = self.query_one("#pane-right", ComparisonPane)

            # Check which viewer is focused
            left_viewer = self.query_one("#viewer-left", FileContentViewer)
            right_viewer = self.query_one("#viewer-right", FileContentViewer)

            if left_viewer.has_focus or left_viewer.has_focus_within:
                left_pane.action_start_search()
            else:
                # Default to right or trigger on whichever had focus last
                right_pane.action_start_search()
        except:
            pass


class SingleArchiveView(Container):
    """View for browsing a single support archive."""

    def __init__(self, archive, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.archive = archive
        self._search_visible = False

    def compose(self) -> ComposeResult:
        """Create the layout."""
        # Search input (initially hidden)
        search_input = Input(placeholder="Search (press Enter)", id="search-input")
        search_input.display = False
        yield search_input

        with Horizontal():
            # Left pane: File tree
            tree = SupportTree(self.archive, id="file-tree")
            tree.border_title = f"Archive: {self.archive.hostname}"
            yield tree

            # Right pane: File viewer
            yield FileContentViewer(id="file-viewer")

    def on_mount(self) -> None:
        """Set focus and pre-load operational-config.json if available."""
        # Try to pre-load operational-config.json
        preferred = Path("operational-config.json")
        full_path = self.archive.path / preferred
        viewer = self.query_one("#file-viewer", FileContentViewer)

        if full_path.exists():
            viewer.load_file(full_path)
            # Focus the viewer since we have content to show
            viewer.focus()
        else:
            # No preferred file, focus tree for navigation
            tree = self.query_one("#file-tree", SupportTree)
            tree.focus()

    def on_tree_node_selected(self, event: Tree.NodeSelected) -> None:
        """Handle file selection in tree."""
        node = event.node
        if node.data and node.data.get("type") == "file":
            file_path = node.data["path"]
            viewer = self.query_one("#file-viewer", FileContentViewer)
            viewer.load_file(file_path)

    def on_file_content_viewer_start_search(self, message: FileContentViewer.StartSearch) -> None:
        """Handle search request from viewer."""
        self.action_start_search()

    def on_file_content_viewer_toggle_line_numbers(self, message: FileContentViewer.ToggleLineNumbers) -> None:
        """Handle line numbers toggle from viewer."""
        # In single view, just toggle the viewer and notify
        viewer = self.query_one("#file-viewer", FileContentViewer)
        new_state = viewer.toggle_line_numbers_internal()
        status = "on" if new_state else "off"
        self.app.notify(f"Line numbers {status}", timeout=1)

    def action_start_search(self) -> None:
        """Show search input."""
        search_input = self.query_one("#search-input", Input)
        search_input.display = True
        self._search_visible = True
        search_input.focus()

    def on_input_submitted(self, event: Input.Submitted) -> None:
        """Handle search submission."""
        if event.input.id == "search-input":
            pattern = event.value
            viewer = self.query_one("#file-viewer", FileContentViewer)
            viewer.search(pattern)

            # Hide search input and refocus viewer
            search_input = self.query_one("#search-input", Input)
            search_input.display = False
            self._search_visible = False
            viewer.focus()

    def on_key(self, event) -> None:
        """Handle escape key to close search input."""
        if event.key == "escape" and self._search_visible:
            search_input = self.query_one("#search-input", Input)
            search_input.display = False
            self._search_visible = False
            viewer = self.query_one("#file-viewer", FileContentViewer)
            viewer.focus()
            event.stop()
            event.prevent_default()


class SupportAnalyzerApp(App):
    """Main TUI application for analyzing support data."""

    CSS = """
    Screen {
        background: $surface;
    }

    #file-tree {
        width: 40%;
        border: solid $primary;
        padding: 1;
    }

    #file-viewer {
        width: 60%;
        border: solid $primary;
        padding: 1;
    }

    SummaryView {
        width: 100%;
        height: 100%;
        border: solid $primary;
        padding: 1;
    }

    .summary-instructions {
        width: 100%;
        height: auto;
        padding: 0 1 1 1;
        color: $text-muted;
    }

    #archive-table {
        width: 100%;
        height: 100%;
    }

    .summary-content {
        width: 100%;
        height: auto;
    }

    .file-content {
        width: 100%;
        height: auto;
    }

    .error {
        color: $error;
    }

    Tree {
        background: $panel;
    }

    /* Comparison view styles */
    .comparison-pane {
        width: 50%;
        border: solid $primary;
        padding: 1;
    }

    .file-selector-label {
        height: 1;
        padding: 0 1;
    }

    .file-selector {
        margin: 0 0 1 0;
    }
    """

    BINDINGS = [
        Binding("q", "quit", "Quit", show=True, priority=True),
        Binding("s", "show_summary", "Summary", show=True),
        Binding("a", "show_analyze", "Analyze", show=True),
        Binding("c", "show_compare", "Compare", show=True),
        Binding("l", "toggle_lock", "Lock Scroll", show=False),  # Only show in compare mode
        Binding("?", "help", "Help", show=True),
    ]

    current_mode = reactive("summary")  # Reactive property for mode switching

    def __init__(self, archives: list, start_mode: str = "summary"):
        super().__init__()
        self.archives = archives  # All available archives
        self.selected_archives = archives  # Currently selected archives for analyze/compare
        self.title = "Infix Support Analyzer"
        # Set initial mode without triggering watch (set directly on the descriptor)
        self._initial_mode = start_mode

    def compose(self) -> ComposeResult:
        """Create the app layout."""
        yield Header()
        # Use initial mode for compose, then set reactive property after
        mode = getattr(self, '_initial_mode', 'summary')
        yield from self._get_view_for_mode(mode)
        yield Footer()

    def on_mount(self) -> None:
        """Called when app is mounted - safe to set reactive properties now."""
        # Now set the reactive property which will trigger watch if changed
        initial_mode = getattr(self, '_initial_mode', 'summary')
        self.current_mode = initial_mode

    def _get_view_for_mode(self, mode: str):
        """Get the appropriate view widget(s) for a mode."""
        if mode == "summary":
            yield SummaryView(self.archives, id="main-view")
        elif mode == "analyze":
            if len(self.selected_archives) > 0:
                yield SingleArchiveView(self.selected_archives[0], id="main-view")
            else:
                yield Label("No archives to analyze", id="main-view")
        elif mode == "compare":
            if len(self.selected_archives) >= 2:
                yield ComparisonView(self.selected_archives[0], self.selected_archives[1], id="main-view")
            else:
                yield Label(f"Comparison requires 2 archives. Found: {len(self.selected_archives)}\n"
                           "Press 's' for summary", id="main-view")
        else:
            yield Label(f"Unknown mode: {mode}", id="main-view")

    def action_show_summary(self) -> None:
        """Switch to summary view."""
        self.current_mode = "summary"

    def action_show_analyze(self) -> None:
        """Switch to analyze view."""
        if len(self.selected_archives) == 0:
            self.notify("No archives selected. Press 's' for summary to select.", severity="warning", timeout=2)
        else:
            self.current_mode = "analyze"

    def action_show_compare(self) -> None:
        """Switch to compare view."""
        if len(self.selected_archives) < 2:
            self.notify(f"Compare requires 2 archives. Selected: {len(self.selected_archives)}. Press 's' for summary.",
                       severity="warning", timeout=3)
        else:
            self.current_mode = "compare"

    def watch_current_mode(self, old_mode: str, new_mode: str) -> None:
        """React to mode changes (Textual reactive watch method)."""
        if old_mode == new_mode:
            return

        # Remove old view
        try:
            old_view = self.query_one("#main-view")
            old_view.remove()
            # Wait for removal to complete before mounting new widget
            self.call_after_refresh(self._mount_new_view, new_mode)
        except:
            # No old view, just mount directly
            self._mount_new_view(new_mode)

    def _mount_new_view(self, mode: str) -> None:
        """Mount the view for the given mode."""
        for widget in self._get_view_for_mode(mode):
            footer = self.query_one(Footer)
            self.mount(widget, before=footer)

    def action_toggle_lock(self) -> None:
        """Toggle scroll lock in comparison view."""
        try:
            comparison_view = self.query_one("#main-view", ComparisonView)
            comparison_view.action_toggle_lock()
        except:
            self.notify("Lock scroll only available in compare mode", timeout=2)

    def action_help(self) -> None:
        """Show help dialog."""
        help_text = (
            "Keybindings:\n"
            "  q = Quit\n"
            "  s = Summary view\n"
            "  a = Analyze view (single archive)\n"
            "  c = Compare view (dual pane)\n"
            "  l = Lock/unlock scroll (compare mode)\n"
            "  Arrow keys = Navigate\n"
            "\n"
            "Compare mode:\n"
            "  Use dropdown menus to select different files in each pane\n"
            "  Files can be different between left and right\n"
            "\n"
            "Text selection: Click and drag, Ctrl+C to copy"
        )
        self.notify(help_text, title="Help", timeout=8)


def launch_tui(archives: list, mode: str = "summary"):
    """Launch the TUI with given archives and mode."""
    app = SupportAnalyzerApp(archives, mode)
    app.run()
