// Dashboard JavaScript functionality
class BotDashboard {
    constructor() {
        this.autoRefreshEnabled = true;
        this.refreshInterval = null;
        this.init();
    }

    init() {
        this.bindEvents();
        this.startAutoRefresh();
    }

    bindEvents() {
        // Auto-refresh toggle
        const autoRefreshCheckbox = document.getElementById('autoRefresh');
        if (autoRefreshCheckbox) {
            autoRefreshCheckbox.addEventListener('change', (e) => {
                this.autoRefreshEnabled = e.target.checked;
                if (this.autoRefreshEnabled) {
                    this.startAutoRefresh();
                } else {
                    this.stopAutoRefresh();
                }
            });
        }

        // Manual refresh button
        const refreshBtn = document.getElementById('refreshBtn');
        if (refreshBtn) {
            refreshBtn.addEventListener('click', () => this.refreshData());
        }

        // Teleport form
        const teleportForm = document.getElementById('teleportForm');
        if (teleportForm) {
            teleportForm.addEventListener('submit', (e) => {
                e.preventDefault();
                this.handleTeleport();
            });
        }

        // Walk form
        const walkForm = document.getElementById('walkForm');
        if (walkForm) {
            walkForm.addEventListener('submit', (e) => {
                e.preventDefault();
                this.handleWalk();
            });
        }

        // Quick action buttons
        const stopFollowBtn = document.getElementById('stopFollowBtn');
        if (stopFollowBtn) {
            stopFollowBtn.addEventListener('click', () => this.stopFollowing());
        }

        const standUpBtn = document.getElementById('standUpBtn');
        if (standUpBtn) {
            standUpBtn.addEventListener('click', () => this.standUp());
        }
    }

    startAutoRefresh() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
        }
        
        this.refreshInterval = setInterval(() => {
            if (this.autoRefreshEnabled) {
                this.refreshData();
            }
        }, 5000); // Refresh every 5 seconds
    }

    stopAutoRefresh() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
    }

    async refreshData() {
        try {
            // Refresh status and logs without full page reload
            await Promise.all([
                this.updateStatus(),
                this.updateLogs()
            ]);
        } catch (error) {
            console.error('Error refreshing data:', error);
        }
    }

    async updateStatus() {
        try {
            const response = await fetch('/api/status');
            const status = await response.json();
            
            // Update status display elements
            this.updateStatusDisplay(status);
        } catch (error) {
            console.error('Error updating status:', error);
        }
    }

    async updateLogs() {
        try {
            const response = await fetch('/api/logs?count=50');
            const logs = await response.json();
            
            // Update logs display
            this.updateLogsDisplay(logs);
        } catch (error) {
            console.error('Error updating logs:', error);
        }
    }

    updateStatusDisplay(status) {
        // Update online status
        const statusElements = document.querySelectorAll('.status-value');
        if (statusElements.length >= 6) {
            // Status
            statusElements[0].textContent = status.isOnline ? 'Online' : 'Offline';
            statusElements[0].className = `status-value ${status.isOnline ? 'online' : 'offline'}`;
            
            // Location
            statusElements[1].textContent = status.currentSim || 'Unknown';
            
            // Position
            statusElements[2].textContent = `${Math.round(status.position.x)}, ${Math.round(status.position.y)}, ${Math.round(status.position.z)}`;
            
            // Following
            statusElements[3].textContent = status.isFollowing ? status.followTarget : 'No';
            
            // Sitting
            statusElements[4].textContent = status.isSitting ? status.sitObject : 'No';
            
            // Last update
            const lastUpdate = new Date(status.lastUpdate);
            statusElements[5].textContent = lastUpdate.toLocaleTimeString();
        }
    }

    updateLogsDisplay(logs) {
        const logsContainer = document.getElementById('logsContainer');
        if (!logsContainer) return;

        logsContainer.innerHTML = '';
        
        logs.forEach(log => {
            const logEntry = document.createElement('div');
            logEntry.className = `log-entry log-${log.type}`;
            
            const timestamp = new Date(log.timestamp);
            const timeStr = timestamp.toLocaleTimeString();
            
            let content = `<strong>${log.avatar}:</strong> ${log.message}`;
            if (log.response) {
                content += `<br><em class="bot-response">Bot: ${log.response}</em>`;
            }
            
            logEntry.innerHTML = `
                <div class="log-timestamp">${timeStr}</div>
                <div class="log-content">${content}</div>
            `;
            
            logsContainer.appendChild(logEntry);
        });

        // Scroll to bottom
        logsContainer.scrollTop = logsContainer.scrollHeight;
    }

    async handleTeleport() {
        const form = document.getElementById('teleportForm');
        const formData = new FormData(form);
        
        const teleportData = {
            region: formData.get('region'),
            x: parseFloat(formData.get('x')) || 128,
            y: parseFloat(formData.get('y')) || 128,
            z: parseFloat(formData.get('z')) || 22
        };

        if (!teleportData.region) {
            this.showMessage('Please enter a region name', 'error');
            return;
        }

        try {
            const response = await fetch('/api/teleport', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(teleportData)
            });

            const result = await response.json();
            this.showMessage(result.message, result.status === 'success' ? 'success' : 'error');
            
            if (result.status === 'success') {
                // Refresh data after successful teleport
                setTimeout(() => this.refreshData(), 2000);
            }
        } catch (error) {
            this.showMessage('Failed to send teleport command', 'error');
        }
    }

    async handleWalk() {
        const form = document.getElementById('walkForm');
        const formData = new FormData(form);
        
        const walkData = {
            x: parseFloat(formData.get('x')),
            y: parseFloat(formData.get('y')),
            z: parseFloat(formData.get('z'))
        };

        if (isNaN(walkData.x) || isNaN(walkData.y) || isNaN(walkData.z)) {
            this.showMessage('Please enter valid coordinates', 'error');
            return;
        }

        try {
            const response = await fetch('/api/walk', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(walkData)
            });

            const result = await response.json();
            this.showMessage(result.message, result.status === 'success' ? 'success' : 'error');
            
            if (result.status === 'success') {
                // Clear form after successful walk command
                form.reset();
                // Refresh data after a short delay
                setTimeout(() => this.refreshData(), 1000);
            }
        } catch (error) {
            this.showMessage('Failed to send walk command', 'error');
        }
    }

    async stopFollowing() {
        try {
            const response = await fetch('/api/stop-following', {
                method: 'POST'
            });

            const result = await response.json();
            this.showMessage(result.message, result.status === 'success' ? 'success' : 'error');
            
            if (result.status === 'success') {
                setTimeout(() => this.refreshData(), 1000);
            }
        } catch (error) {
            this.showMessage('Failed to stop following', 'error');
        }
    }

    async standUp() {
        try {
            const response = await fetch('/api/stand', {
                method: 'POST'
            });

            const result = await response.json();
            this.showMessage(result.message, result.status === 'success' ? 'success' : 'error');
            
            if (result.status === 'success') {
                setTimeout(() => this.refreshData(), 1000);
            }
        } catch (error) {
            this.showMessage('Failed to stand up', 'error');
        }
    }

    showMessage(message, type = 'info') {
        const messagesContainer = document.getElementById('statusMessages');
        if (!messagesContainer) return;

        const messageElement = document.createElement('div');
        messageElement.className = `status-message ${type}`;
        messageElement.textContent = message;

        messagesContainer.appendChild(messageElement);

        // Auto-remove message after 5 seconds
        setTimeout(() => {
            if (messageElement.parentNode) {
                messageElement.parentNode.removeChild(messageElement);
            }
        }, 5000);
    }

    // Utility method to format timestamps
    formatTimestamp(timestamp) {
        const date = new Date(timestamp);
        return date.toLocaleTimeString();
    }

    // Method to handle keyboard shortcuts
    handleKeyboardShortcuts(event) {
        // Ctrl/Cmd + R for manual refresh
        if ((event.ctrlKey || event.metaKey) && event.key === 'r') {
            event.preventDefault();
            this.refreshData();
        }
        
        // Ctrl/Cmd + T for focus on teleport region input
        if ((event.ctrlKey || event.metaKey) && event.key === 't') {
            event.preventDefault();
            const regionInput = document.getElementById('tpRegion');
            if (regionInput) {
                regionInput.focus();
            }
        }
    }
}

// Initialize dashboard when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    const dashboard = new BotDashboard();
    
    // Add keyboard shortcut support
    document.addEventListener('keydown', (event) => {
        dashboard.handleKeyboardShortcuts(event);
    });
    
    // Handle page visibility changes (pause auto-refresh when tab is not visible)
    document.addEventListener('visibilitychange', () => {
        const autoRefreshCheckbox = document.getElementById('autoRefresh');
        if (document.hidden) {
            dashboard.stopAutoRefresh();
        } else if (autoRefreshCheckbox && autoRefreshCheckbox.checked) {
            dashboard.startAutoRefresh();
        }
    });
});

// Add some utility functions for potential future use
window.BotDashboard = {
    // Export useful methods for console access
    refreshData: () => {
        if (window.dashboardInstance) {
            window.dashboardInstance.refreshData();
        }
    },
    
    showMessage: (message, type) => {
        if (window.dashboardInstance) {
            window.dashboardInstance.showMessage(message, type);
        }
    }
};
