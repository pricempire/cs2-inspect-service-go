document.addEventListener("DOMContentLoaded", function () {
	// DOM elements
	const inspectForm = document.getElementById("inspectForm");
	const inspectLinkInput = document.getElementById("inspectLink");
	const refreshCheckbox = document.getElementById("refreshCheckbox");
	const errorMessage = document.getElementById("errorMessage");
	const loadingElement = document.getElementById("loading");
	const resultContainer = document.getElementById("resultContainer");
	const jsonViewTab = document.getElementById("jsonViewTab");
	const visualViewTab = document.getElementById("visualViewTab");
	const jsonViewContent = document.getElementById("jsonViewContent");
	const visualViewContent = document.getElementById("visualViewContent");
	const jsonOutput = document.getElementById("jsonOutput");
	const copyJsonBtn = document.getElementById("copyJsonBtn");

	// Tab switching
	if (jsonViewTab && visualViewTab) {
		jsonViewTab.addEventListener("click", function () {
			setActiveTab("json");
		});

		visualViewTab.addEventListener("click", function () {
			setActiveTab("visual");
		});
	}

	// Copy JSON button
	if (copyJsonBtn) {
		copyJsonBtn.addEventListener("click", function () {
			copyToClipboard(jsonOutput.textContent);
			showToast("JSON copied to clipboard");
		});
	}

	// Form submission
	if (inspectForm) {
		inspectForm.addEventListener("submit", function (event) {
			event.preventDefault();
			handleFormSubmit();
		});
	}

	// Input validation
	if (inspectLinkInput) {
		inspectLinkInput.addEventListener("input", function () {
			if (this.value.trim()) {
				this.classList.remove("error");
				errorMessage.style.display = "none";
			}
		});
	}

	/**
	 * Handle form submission
	 */
	function handleFormSubmit() {
		const inspectLink = inspectLinkInput.value.trim();

		// Validate input
		if (!inspectLink) {
			showError("Please enter an inspect link");
			return;
		}

		// Validate steam inspect URL format
		const steamUrlPattern =
			/^(?:steam:\/\/rungame\/730\/|https?:\/\/(?:www\.)?steamcommunity\.com\/market\/listings\/730\/.*[?&]inspectlink=steam:\/\/rungame\/730\/)/i;

		// Check for S, A, D, M parameters format
		const paramPattern = /[?&](?:s=\d+&a=\d+&d=\d+&m=\d+|[SADM]=\d+)/i;

		if (!steamUrlPattern.test(inspectLink) && !paramPattern.test(inspectLink)) {
			showError("Please enter a valid CS2 inspect link");
			return;
		}

		// Show loading indicator
		loadingElement.style.display = "flex";
		resultContainer.style.display = "none";

		// Build the API URL
		let apiUrl = "/inspect?link=" + encodeURIComponent(inspectLink);

		// Add refresh parameter if checked
		if (refreshCheckbox && refreshCheckbox.checked) {
			apiUrl += "&refresh=true";
		}

		// Make the API request
		fetchInspectData(apiUrl);
	}

	/**
	 * Fetch data from the inspect API
	 * @param {string} url - The API URL
	 */
	function fetchInspectData(url) {
		fetch(url)
			.then((response) => {
				if (!response.ok) {
					throw new Error("Network response was not ok: " + response.status);
				}
				return response.json();
			})
			.then((data) => {
				// Hide loading indicator
				loadingElement.style.display = "none";

				// Display the result
				displayResult(data);
			})
			.catch((error) => {
				// Hide loading indicator
				loadingElement.style.display = "none";

				// Show error message
				showError("Error fetching data: " + error.message);
			});
	}

	/**
	 * Display the API result
	 * @param {Object} data - The API response data
	 */
	function displayResult(data) {
		// Show the result container
		resultContainer.style.display = "block";

		// Format and display JSON view
		const formattedJson = JSON.stringify(data, null, 2);
		jsonOutput.textContent = formattedJson;

		// Display visual view if the request was successful
		if (data.success && data.iteminfo) {
			renderVisualView(data.iteminfo);
			setActiveTab("visual"); // Default to visual view
		} else {
			// If there was an error, show JSON view
			setActiveTab("json");

			// Display error in visual view
			visualViewContent.innerHTML = `
                <div class="error-container">
                    <h3>Error</h3>
                    <p>${data.error || "Unknown error occurred"}</p>
                </div>
            `;
		}
	}

	/**
	 * Render the visual representation of the item
	 * @param {Object} item - The item info object
	 */
	function renderVisualView(item) {
		let html = `
            <div class="item-header">
                ${
									item.image
										? `<img src="${item.image}" alt="${
												item.market_hash_name || "CS2 Item"
										  }" class="item-image">`
										: ""
								}
                <div class="item-details">
                    <h3 class="item-name">${
											item.market_hash_name || "Unknown Item"
										}</h3>
                    <div class="item-badges">
                        ${
													item.stattrak
														? '<span class="badge stattrak">StatTrak™</span>'
														: ""
												}
                        ${
													item.souvenir
														? '<span class="badge souvenir">Souvenir</span>'
														: ""
												}
                        ${
													item.wear_name
														? `<span class="badge wear">${item.wear_name}</span>`
														: ""
												}
                    </div>
                </div>
            </div>
            
            <div class="item-properties">
                <div class="property-group">
                    <h4>General Information</h4>
                    ${createPropertyRow(
											"Float Value",
											formatFloat(item.floatvalue)
										)}
                    ${createPropertyRow("Paint Seed", item.paintseed)}
                    ${
											item.pattern
												? createPropertyRow("Pattern", item.pattern)
												: ""
										}
                    ${item.phase ? createPropertyRow("Phase", item.phase) : ""}
                    ${createPropertyRow(
											"Quality",
											getQualityName(item.quality)
										)}
                    ${createPropertyRow("Rarity", getRarityName(item.rarity))}
                    ${createPropertyRow("Origin", getOriginName(item.origin))}
                </div>
                
                <div class="property-group">
                    <h4>Technical Details</h4>
                    ${createPropertyRow("Def Index", item.defindex)}
                    ${createPropertyRow("Paint Index", item.paintindex)}
                    ${
											item.min !== undefined
												? createPropertyRow("Min Float", formatFloat(item.min))
												: ""
										}
                    ${
											item.max !== undefined
												? createPropertyRow("Max Float", formatFloat(item.max))
												: ""
										}
                    ${
											item.rank
												? createPropertyRow(
														"Float Rank",
														`#${item.rank}${
															item.totalcount ? ` / ${item.totalcount}` : ""
														}`
												  )
												: ""
										}
                </div>
            </div>
        `;

		// Add stickers section if there are stickers
		if (item.stickers && item.stickers.length > 0) {
			html += `
                <div class="stickers-container">
                    <h4>Stickers (${item.stickers.length})</h4>
                    <div class="stickers-list">
                        ${item.stickers
													.map(
														(sticker, index) => `
                            <div class="sticker-item">
                                <span>${
																	sticker.name ||
																	`Sticker #${sticker.sticker_id}`
																}</span>
                                ${
																	sticker.wear !== null &&
																	sticker.wear !== undefined
																		? `<span class="sticker-wear">Wear: ${formatFloat(
																				sticker.wear
																		  )}</span>`
																		: ""
																}
                                <span class="sticker-position">Position: ${getPositionName(
																	sticker.slot
																)}</span>
                            </div>
                        `
													)
													.join("")}
                    </div>
                </div>
            `;
		}

		// Add keychains section if there are keychains
		if (item.keychains && item.keychains.length > 0) {
			html += `
                <div class="keychains-container">
                    <h4>Keychains (${item.keychains.length})</h4>
                    <div class="keychains-list">
                        ${item.keychains
													.map(
														(keychain, index) => `
                            <div class="keychain-item">
                                <span>${
																	keychain.name ||
																	`Keychain #${keychain.sticker_id}`
																}</span>
                            </div>
                        `
													)
													.join("")}
                    </div>
                </div>
            `;
		}

		// Add custom name if present
		if (item.customname) {
			html += `
                <div class="custom-name-container">
                    <h4>Custom Name</h4>
                    <p>"${item.customname}"</p>
                </div>
            `;
		}

		visualViewContent.innerHTML = html;
	}

	/**
	 * Create a property row HTML
	 * @param {string} label - The property label
	 * @param {string|number} value - The property value
	 * @returns {string} HTML for the property row
	 */
	function createPropertyRow(label, value) {
		if (value === undefined || value === null) return "";
		return `
            <div class="property">
                <span class="property-label">${label}:</span>
                <span class="property-value">${value}</span>
            </div>
        `;
	}

	/**
	 * Format a float value to a readable string
	 * @param {number} value - The float value
	 * @returns {string} Formatted float value
	 */
	function formatFloat(value) {
		if (value === undefined || value === null) return "N/A";
		return value.toFixed(10).replace(/\.?0+$/, "");
	}

	/**
	 * Get the quality name from the quality ID
	 * @param {number} qualityId - The quality ID
	 * @returns {string} Quality name
	 */
	function getQualityName(qualityId) {
		const qualities = {
			0: "Normal",
			1: "Genuine",
			2: "Vintage",
			3: "Unusual",
			4: "Unique",
			5: "Community",
			6: "Developer",
			7: "Self-Made",
			8: "Customized",
			9: "Strange",
			10: "Completed",
			11: "Tournament",
			12: "Valve",
		};
		return qualities[qualityId] || `Unknown (${qualityId})`;
	}

	/**
	 * Get the rarity name from the rarity ID
	 * @param {number} rarityId - The rarity ID
	 * @returns {string} Rarity name
	 */
	function getRarityName(rarityId) {
		const rarities = {
			0: "Common",
			1: "Uncommon",
			2: "Rare",
			3: "Mythical",
			4: "Legendary",
			5: "Ancient",
			6: "Immortal",
			7: "Arcana",
			8: "Contraband",
		};
		return rarities[rarityId] || `Unknown (${rarityId})`;
	}

	/**
	 * Get the origin name from the origin ID
	 * @param {number} originId - The origin ID
	 * @returns {string} Origin name
	 */
	function getOriginName(originId) {
		const origins = {
			0: "Unknown",
			1: "Purchased",
			2: "Crafted",
			3: "Traded",
			4: "Dropped",
			5: "Market",
			6: "Store",
			7: "Gift",
			8: "Traded Up",
			9: "CD Key",
			10: "Collection Reward",
			11: "Preview",
			12: "Tournament Drop",
		};
		return origins[originId] || `Unknown (${originId})`;
	}

	/**
	 * Get the position name from the slot ID
	 * @param {number} slotId - The slot ID
	 * @returns {string} Position name
	 */
	function getPositionName(slotId) {
		const positions = {
			0: "Position 1",
			1: "Position 2",
			2: "Position 3",
			3: "Position 4",
		};
		return positions[slotId] || `Position ${slotId + 1}`;
	}

	/**
	 * Set the active tab
	 * @param {string} tabName - The tab name ('json' or 'visual')
	 */
	function setActiveTab(tabName) {
		if (tabName === "json") {
			jsonViewTab.classList.add("active");
			visualViewTab.classList.remove("active");
			jsonViewContent.classList.add("active");
			visualViewContent.classList.remove("active");
		} else {
			jsonViewTab.classList.remove("active");
			visualViewTab.classList.add("active");
			jsonViewContent.classList.remove("active");
			visualViewContent.classList.add("active");
		}
	}

	/**
	 * Show an error message
	 * @param {string} message - The error message
	 */
	function showError(message) {
		inspectLinkInput.classList.add("error");
		errorMessage.textContent = message;
		errorMessage.style.display = "block";
	}

	/**
	 * Copy text to clipboard
	 * @param {string} text - The text to copy
	 */
	function copyToClipboard(text) {
		const textarea = document.createElement("textarea");
		textarea.value = text;
		document.body.appendChild(textarea);
		textarea.select();
		document.execCommand("copy");
		document.body.removeChild(textarea);
	}

	/**
	 * Show a toast notification
	 * @param {string} message - The toast message
	 */
	function showToast(message) {
		const toast = document.createElement("div");
		toast.className = "toast";
		toast.textContent = message;
		document.body.appendChild(toast);

		// Trigger animation
		setTimeout(() => {
			toast.classList.add("show");
		}, 10);

		// Remove after 3 seconds
		setTimeout(() => {
			toast.classList.remove("show");
			setTimeout(() => {
				document.body.removeChild(toast);
			}, 300);
		}, 3000);
	}
});
