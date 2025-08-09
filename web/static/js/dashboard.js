// Enhanced Dashboard JavaScript functionality with avatar tracking
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

        const toggleLlamaBtn = document.getElementById('toggleLlamaBtn');
        if (toggleLlamaBtn) {
            toggleLlamaBtn.addEventListener('click', () => this.toggleLlama());
        }

        // Macro management buttons
        this.bindMacroEvents();
    }

    bindMacroEvents() {
        // Make functions globally available for onclick handlers
        window.playMacro = (name) => this.playMacro(name);
        window.deleteMacro = (name) => this.deleteMacro(name);
        window.setIdleBehavior = (name) => this.setIdleBehavior(name);
        window.unsetIdleBehavior = (name) => this.unsetIdleBehavior(name);
        window.setAutoGreetMacro = (name) => this.setAutoGreetMacro(name);
        window.unsetAutoGreetMacro = (name) => this.unsetAutoGreetMacro(name);
        window.greetAvatar = (name) => this.greetAvatar(name);
        window.teleportToAvatar = (name, x, y, z) => this.teleportToAvatar(name, x, y, z);
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
            // Refresh status, logs, and avatars without full page reload
            await Promise.all([
                this.updateStatus(),
                this.updateLogs(),
                this.updateAvatars()
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

    async updateAvatars() {
        try {
            const response = await fetch('/api/avatars');
            const avatars = await response.json();
            
            // Update avatar display
            this.updateAvatarsDisplay(avatars);
        } catch (error) {
            console.error('Error updating avatars:', error);
        }
    }

    updateStatusDisplay(status) {
        // Update status elements
        const statusElements = document.querySelectorAll('.status-value');
        if (statusElements.length >= 9) {
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
            
            // AI Chat
            statusElements[5].textContent = status.llamaEnabled ? 'Enabled' : 'Disabled';
            statusElements[5].className = `status-value ${status.llamaEnabled ? 'online' : 'offline'}`;
            
            // Bot Status
            statusElements[6].textContent = status.isIdle ? 'Idle' : 'Active';
            statusElements[6].className = `status-value ${status.isIdle ? 'idle' : 'active'}`;
            
            // Auto-Greet
            statusElements[7].textContent = status.autoGreetEnabled ? status.autoGreetMacro : 'Disabled';
            statusElements[7].className = `status-value ${status.autoGreetEnabled ? 'online' : 'offline'}`;
            
            // Last update
            const lastUpdate = new Date(status.lastUpdate);
            statusElements[8].textContent = lastUpdate.toLocaleTimeString();
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

    updateAvatarsDisplay(avatars) {
        const avatarsSection = document.querySelector('.nearby-avatars-section');
        const noAvatarsDiv = document.querySelector('.no-avatars');
        
        if (Object.keys(avatars).length === 0) {
            if (avatarsSection) avatarsSection.style.display = 'none';
            if (noAvatarsDiv) noAvatarsDiv.style.display = 'block';
            return;
        }
        
        if (avatarsSection) avatarsSection.style.display = 'block';
        if (noAvatarsDiv) noAvatarsDiv.style.display = 'none';
        
        const avatarsList = document.querySelector('.avatars-list');
        if (!avatarsList) return;
        
        avatarsList.innerHTML = '';
        
        Object.entries(avatars).forEach(([name, avatar]) => {
            const avatarDiv = document.createElement('div');
            avatarDiv.className = `avatar-item ${avatar.isGreeted ? 'greeted' : 'new'}`;
            
            const firstSeenTime = new Date(avatar.firstSeen).toLocaleTimeString();
            const lastSeenTime = new Date(avatar.lastSeen).toLocaleTimeString();
            const timeSince = this.formatTimeSince(new Date(avatar.lastSeen));
            
            avatarDiv.innerHTML = `
                <div class="avatar-info">
                    <h4>${name} ${!avatar.isGreeted ? '<span class="new-badge">NEW</span>' : ''}</h4>
                    <p class="avatar-position">Position: ${Math.round(avatar.position.x)}, ${Math.round(avatar.position.y)}, ${Math.round(avatar.position.z)}</p>
                    <p class="avatar-times">
                        First seen: ${firstSeenTime}<br>
                        Last seen: ${lastSeenTime} (${timeSince} ago)
                    </p>
                </div>
                <div class="avatar-actions">
                    ${!avatar.isGreeted ? `<button class="btn btn-sm btn-primary" onclick="greetAvatar('${name}')">Greet Now</button>` : ''}
                    <button class="btn btn-sm btn-secondary" onclick="teleportToAvatar('${name}', ${avatar.position.x}, ${avatar.position.y}, ${avatar.position.z})">Teleport To</button>
                </div>
            `;
            
            avatarsList.appendChild(avatarDiv);
        });
    }

    formatTimeSince(date) {
        const seconds = Math.floor((new Date() - date) / 1000);
        
        if (seconds < 60) return `${seconds}s`;
        
        const minutes = Math.floor(seconds / 60);
        if (minutes < 60) return `${minutes}m`;
        
        const hours = Math.floor(minutes / 60);
        return `${hours}h`;
    }

    // Avatar-specific functions
    async greetAvatar(avatarName) {
        try {
            const autoGreetResponse = await fetch('/api/autogreet');
            const autoGreetConfig = await autoGreetResponse.json();
            
            if (!autoGreetConfig.enabled || !autoGreetConfig.macroName) {
                this.showMessage('No auto-greet macro configured', 'error');
                return;
            }
            
            const response = await fetch(`/api/macros/play/${encodeURIComponent(autoGreetConfig.macroName)}`, {
                method: 'POST'
            });
            
            const result = await response.json();
            this.showMessage(`${result.status === 'success' ? 'Greeting' : 'Failed to greet'} ${avatarName}`, result.status === 'success' ? 'success' : 'error');
        } catch (error) {
            this.showMessage('Failed to greet avatar', 'error');
        }
    }

    async teleportToAvatar(avatarName, x, y, z) {
        try {
            const statusResponse = await fetch('/api/status');
            const status = await statusResponse.json();
            
            const teleportData = {
                region: status.currentSim,
                x: x,
                y: y,
                z: z + 2 // Teleport slightly above the avatar
            };
            
            const response = await fetch('/api/teleport', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(teleportData)
            });

            const result = await response.json();
            this.showMessage(`${result.status === 'success' ? 'Teleporting near' : 'Failed to teleport to'} ${avatarName}`, result.status === 'success' ? 'success' : 'error');
        } catch (error) {
            this.showMessage('Failed to teleport to avatar', 'error');
        }
    }

    // Macro management functions
    async playMacro(name) {
        try {
            const response = await fetch(`/api/macros/play/${encodeURIComponent(name)}`, {
                method: 'POST'
            });
            const result = await response.json();
            this.showMessage(result.message, result.status === 'success' ? 'success' : 'error');
        } catch (error) {
            this.showMessage('Failed to play macro', 'error');
        }
    }

    async deleteMacro(name) {
        if (!confirm(`Are you sure you want to delete macro '${name}'?`)) {
            return;
        }
        
        try {
            const response = await fetch(`/api/macros/delete/${encodeURIComponent(name)}`, {
                method: 'DELETE'
            });
            const result = await response.json();
            this.showMessage(result.message, result.status === 'success' ? 'success' : 'error');
            
            if (result.status === 'success') {
                setTimeout(() => window.location.reload(), 1000);
            }
        } catch (error) {
            this.showMessage('Failed to delete macro', 'error');
        }
    }

    async setIdleBehavior(name) {
        try {
            const response = await fetch(`/api/macros/idle/${encodeURIComponent(name)}`, {
                method: 'POST'
            });
            const result = await response.json();
            this.showMessage(result.message, result.status === 'success' ? 'success' : 'error');
            
            if (result.status === 'success') {
                setTimeout(() => window.location.reload(), 1000);
            }
        } catch (error) {
            this.showMessage('Failed to set idle behavior', 'error');
        }
    }

    async unsetIdleBehavior(name) {
        try {
            const response = await fetch(`/api/macros/idle/${encodeURIComponent(name)}`, {
                method: 'DELETE'
            });
            const result = await response.json();
            this.showMessage(result.message, result.status === 'success' ? 'success' : 'error');
            
            if (result.status === 'success') {
                setTimeout(() => window.location.reload(), 1000);
            }
        } catch (error) {
            this.showMessage('Failed to unset idle behavior', 'error');
        }
    }

    async setAutoGreetMacro(name) {
        try {
            const response = await fetch(`/api/macros/autogreet/${encodeURIComponent(name)}`, {
                method: 'POST'
            });
            const result = await response.json();
            this.showMessage(result.message, result.status === 'success' ? 'success' : 'error');
            
            if (result.status === 'success') {
                setTimeout(() => window.location.reload(), 1000);
            }
        } catch (error) {
            this.showMessage('Failed to set auto-greet macro', 'error');
        }
    }

    async unsetAutoGreetMacro(name) {
        try {
            const response = await fetch(`/api/macros/autogreet/${encodeURIComponent(name)}`, {
                method: 'DELETE'
            });
            const result = await response.json();
            this.showMessage(result.message, result.status === 'success' ? 'success' : 'error');
            
            if (result.status === 'success') {
                setTimeout(() => window.location.reload(), 1000);
            }
        } catch (error) {
            this.showMessage('Failed to unset auto-greet macro', 'error');
        }
    }

    // Existing functions
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

    async toggleLlama() {
        try {
            const response = await fetch('/api/toggle-llama', {
                method: 'POST'
            });

            const result = await response.json();
            this.showMessage(result.message, result.status === 'success' ? 'success' : 'error');
            
            if (result.status === 'success') {
                setTimeout(() => this.refreshData(), 1000);
            }
        } catch (error) {
            this.showMessage('Failed to toggle Llama', 'error');
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

        // Ctrl/Cmd + A for focus on avatar list
        if ((event.ctrlKey || event.metaKey) && event.key === 'a') {
            event.preventDefault();
            const avatarSection = document.querySelector('.nearby-avatars-section');
            if (avatarSection) {
                avatarSection.scrollIntoView({ behavior: 'smooth' });
            }
        }
    }
}

// Initialize dashboard when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    const dashboard = new BotDashboard();
    
    // Make dashboard instance globally available
    window.dashboard = dashboard;
    
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

    // Auto-greet configuration event listeners
    const enableAutoGreetBtn = document.getElementById('enableAutoGreetBtn');
    const disableAutoGreetBtn = document.getElementById('disableAutoGreetBtn');
    const autoGreetMacroSelect = document.getElementById('autoGreetMacroSelect');
    
    if (enableAutoGreetBtn && autoGreetMacroSelect) {
        enableAutoGreetBtn.addEventListener('click', async function() {
            const selectedMacro = autoGreetMacroSelect.value;
            if (!selectedMacro) {
                dashboard.showMessage('Please select a macro first', 'error');
                return;
            }
            
            const requestData = {
                enabled: true,
                macroName: selectedMacro
            };
            
            try {
                const response = await fetch('/api/autogreet', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify(requestData)
                });
                
                const data = await response.json();
                dashboard.showMessage(data.message, data.status === 'success' ? 'success' : 'error');
                
                if (data.status === 'success') {
                    setTimeout(() => window.location.reload(), 1000);
                }
            } catch (error) {
                dashboard.showMessage('Failed to enable auto-greet', 'error');
            }
        });
    }
    
    if (disableAutoGreetBtn) {
        disableAutoGreetBtn.addEventListener('click', async function() {
            try {
                const response = await fetch('/api/autogreet', {
                    method: 'DELETE'
                });
                
                const data = await response.json();
                dashboard.showMessage(data.message, data.status === 'success' ? 'success' : 'error');
                
                if (data.status === 'success') {
                    setTimeout(() => window.location.reload(), 1000);
                }
            } catch (error) {
                dashboard.showMessage('Failed to disable auto-greet', 'error');
            }
        });
    }
});

// Export useful methods for console access
window.BotDashboard = {
    refreshData: () => {
        if (window.dashboard) {
            window.dashboard.refreshData();
        }
    },
    
    showMessage: (message, type) => {
        if (window.dashboard) {
            window.dashboard.showMessage(message, type);
        }
    },

    greetAllNewAvatars: async () => {
        if (window.dashboard) {
            try {
                const response = await fetch('/api/avatars');
                const avatars = await response.json();
                
                const newAvatars = Object.entries(avatars).filter(([name, avatar]) => !avatar.isGreeted);
                
                if (newAvatars.length === 0) {
                    window.dashboard.showMessage('No new avatars to greet', 'info');
                    return;
                }
                
                for (const [name, avatar] of newAvatars) {
                    await window.dashboard.greetAvatar(name);
                    // Small delay between greetings to avoid spam
                    await new Promise(resolve => setTimeout(resolve, 2000));
                }
                
                window.dashboard.showMessage(`Greeted ${newAvatars.length} new avatars`, 'success');
            } catch (error) {
                window.dashboard.showMessage('Failed to greet new avatars', 'error');
            }
        }
    }
};
