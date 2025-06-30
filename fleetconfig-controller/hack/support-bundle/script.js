const navigationStack = {
    hub: ['/hub/'],
    spoke: ['/spoke/']
};

const paneStates = {
    hub: { minimized: false, wrapped: false },
    spoke: { minimized: false, wrapped: false }
};

function toggleMinimize(pane) {
    const paneElement = document.getElementById(`${pane}-pane`);
    const minimizeButton = document.getElementById(`${pane}-minimize`);
    
    paneStates[pane].minimized = !paneStates[pane].minimized;
    
    if (paneStates[pane].minimized) {
        paneElement.classList.add('minimized');
        minimizeButton.textContent = '+';
        minimizeButton.title = 'Maximize';
        
        // Make the entire header clickable when minimized
        const paneHeader = paneElement.querySelector('.pane-header');
        paneHeader.style.cursor = 'pointer';
        paneHeader.title = 'Click to maximize';
        paneHeader.onclick = (e) => {
            e.stopPropagation();
            toggleMinimize(pane);
        };
    } else {
        paneElement.classList.remove('minimized');
        minimizeButton.textContent = '−';
        minimizeButton.title = 'Minimize';
        
        // Remove header click handler when expanded
        const paneHeader = paneElement.querySelector('.pane-header');
        paneHeader.style.cursor = 'default';
        paneHeader.title = '';
        paneHeader.onclick = null;
    }
}

function toggleWrap(pane) {
    const wrapButton = document.getElementById(`${pane}-wrap`);
    const container = document.getElementById(`${pane}-content`);
    
    paneStates[pane].wrapped = !paneStates[pane].wrapped;
    
    // Update button text and title
    if (paneStates[pane].wrapped) {
        wrapButton.textContent = 'Unwrap';
        wrapButton.title = 'Disable word wrap';
    } else {
        wrapButton.textContent = 'Wrap';
        wrapButton.title = 'Enable word wrap';
    }
    
    // Apply wrap state to any existing file content
    const fileContent = container.querySelector('.file-content');
    if (fileContent) {
        updateFileContentWrapState(fileContent, paneStates[pane].wrapped);
    }
}

function updateFileContentWrapState(fileContentElement, wrapped) {
    fileContentElement.classList.remove('wrapped', 'unwrapped');
    fileContentElement.classList.add(wrapped ? 'wrapped' : 'unwrapped');
}

async function loadDirectoryContents(directory, containerId, path = `/${directory}/`) {
    const container = document.getElementById(containerId);
    const breadcrumb = document.getElementById(`${directory}-breadcrumb`);
    const backButton = document.getElementById(`${directory}-back`);
    
    try {
        container.innerHTML = '<div class="loading">Loading...</div>';
        
        const response = await fetch(path);
        const html = await response.text();
        
        // Parse the directory listing HTML
        const parser = new DOMParser();
        const doc = parser.parseFromString(html, 'text/html');
        const links = doc.querySelectorAll('a[href]');
        
        const fileList = document.createElement('ul');
        fileList.className = 'file-list';
        
        // Add directory info
        const dirInfo = document.createElement('div');
        dirInfo.className = 'directory-info';
        dirInfo.textContent = `Directory: ${path}`;
        
        links.forEach(link => {
            const href = link.getAttribute('href');
            // Skip parent directory link and self
            if (href === '../' || href === './') return;
            
            const listItem = document.createElement('li');
            listItem.className = 'file-item';
            
            const fileLink = document.createElement('div');
            fileLink.className = 'file-link';
            
            const icon = document.createElement('span');
            icon.className = 'file-icon';
            icon.textContent = href.endsWith('/') ? '📁' : '📄';
            
            const name = document.createElement('span');
            name.className = 'file-name';
            name.textContent = decodeURIComponent(href);
            
            fileLink.appendChild(icon);
            fileLink.appendChild(name);
            
            // Add click handler
            fileLink.onclick = (e) => {
                e.preventDefault();
                const fullPath = path + href;
                if (href.endsWith('/')) {
                    // It's a directory
                    navigationStack[directory].push(fullPath);
                    loadDirectoryContents(directory, containerId, fullPath);
                } else {
                    // It's a file
                    loadFileContent(directory, containerId, fullPath);
                }
            };
            
            listItem.appendChild(fileLink);
            fileList.appendChild(listItem);
        });
        
        container.innerHTML = '';
        container.appendChild(dirInfo);
        container.appendChild(fileList);
        
        // Update breadcrumb and back button
        updateNavigation(directory, path);
        
    } catch (error) {
        console.error(`Error loading ${directory}:`, error);
        container.innerHTML = `<div class="error">Error loading ${directory} contents</div>`;
    }
}

async function loadFileContent(directory, containerId, filePath) {
    const container = document.getElementById(containerId);
    
    try {
        container.innerHTML = '<div class="loading">Loading file...</div>';
        
        const response = await fetch(filePath);
        const content = await response.text();
        
        const fileContainer = document.createElement('div');
        
        // File info
        const fileInfo = document.createElement('div');
        fileInfo.className = 'directory-info';
        fileInfo.textContent = `File: ${filePath}`;
        
        // File content
        const fileContent = document.createElement('div');
        fileContent.className = 'file-content';
        fileContent.textContent = content;
        
        // Apply current wrap state
        updateFileContentWrapState(fileContent, paneStates[directory].wrapped);
        
        fileContainer.appendChild(fileInfo);
        fileContainer.appendChild(fileContent);
        
        container.innerHTML = '';
        container.appendChild(fileContainer);
        
        // Update navigation
        updateNavigation(directory, filePath);
        
    } catch (error) {
        console.error(`Error loading file:`, error);
        container.innerHTML = `<div class="error">Error loading file content</div>`;
    }
}

function updateNavigation(directory, currentPath) {
    const breadcrumb = document.getElementById(`${directory}-breadcrumb`);
    const backButton = document.getElementById(`${directory}-back`);
    
    // Show current path in breadcrumb
    const pathParts = currentPath.split('/').filter(p => p);
    breadcrumb.textContent = pathParts.length > 1 ? `/${pathParts.slice(1).join('/')}` : '/';
    
    // Show/hide back button
    if (navigationStack[directory].length > 1) {
        backButton.style.display = 'block';
    } else {
        backButton.style.display = 'none';
    }
}

function navigateBack(directory) {
    if (navigationStack[directory].length > 1) {
        navigationStack[directory].pop(); // Remove current
        const previousPath = navigationStack[directory][navigationStack[directory].length - 1];
        loadDirectoryContents(directory, `${directory}-content`, previousPath);
    }
}

// Load both directories when the page loads
document.addEventListener('DOMContentLoaded', () => {
    loadDirectoryContents('hub', 'hub-content');
    loadDirectoryContents('spoke', 'spoke-content');
}); 