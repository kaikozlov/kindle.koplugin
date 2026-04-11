package kfx

// Notebook / scribe: Calibre KFX_EPUB_Notebook (yj_to_epub_notebook.py). process_section branches to
// process_scribe_* when nmdl.* keys are present; stubs keep the Python call graph discoverable from Go.

// Port of KFX_EPUB_Content.process_scribe_notebook_page_section (yj_to_epub_notebook.py) — not used until a scribe fixture drives it.
func processScribeNotebookPageSection(section map[string]interface{}, pageTemplate map[string]interface{}, sectionName string, seq int) bool {
	_, _, _, _ = section, pageTemplate, sectionName, seq
	return false
}

// Port of KFX_EPUB_Content.process_scribe_notebook_template_section (yj_to_epub_notebook.py).
func processScribeNotebookTemplateSection(section map[string]interface{}, pageTemplate map[string]interface{}, sectionName string) bool {
	_, _, _ = section, pageTemplate, sectionName
	return false
}
