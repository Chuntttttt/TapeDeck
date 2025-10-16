// Print mode interactions for sticker generation
(function() {
  'use strict';

  // Only run if we're on the mappings page in print mode
  const printForm = document.getElementById('print-form');
  if (!printForm) return;

  const selectAllBtn = document.getElementById('select-all');
  const deselectAllBtn = document.getElementById('deselect-all');
  const generateBtn = document.getElementById('generate-btn');
  const checkboxes = document.querySelectorAll('.sticker-checkbox');

  // Select all checkboxes
  if (selectAllBtn) {
    selectAllBtn.addEventListener('click', function() {
      checkboxes.forEach(cb => cb.checked = true);
    });
  }

  // Deselect all checkboxes
  if (deselectAllBtn) {
    deselectAllBtn.addEventListener('click', function() {
      checkboxes.forEach(cb => cb.checked = false);
    });
  }

  // Handle form submission
  if (printForm) {
    printForm.addEventListener('submit', function(e) {
      // Check if at least one checkbox is selected
      const selectedCount = Array.from(checkboxes).filter(cb => cb.checked).length;

      if (selectedCount === 0) {
        e.preventDefault();
        alert('Please select at least one mapping to print.');
        return;
      }

      // Change button text while generating
      generateBtn.textContent = 'Generating...';
      generateBtn.disabled = true;
    });
  }

  // Update button text after successful download
  // Note: This only works if the response triggers a download, not a page navigation
  window.addEventListener('focus', function() {
    if (generateBtn && generateBtn.textContent === 'Generating...') {
      generateBtn.textContent = '✅ Done';
      generateBtn.disabled = false;

      setTimeout(function() {
        generateBtn.textContent = 'Generate PDF';
      }, 3000);
    }
  });
})();
