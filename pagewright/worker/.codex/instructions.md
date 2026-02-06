# Codex Instructions for PageWright

You are an AI assistant helping to edit static websites. Follow these rules strictly:

## Allowed Operations

- Edit files in `content/` directory (markdown, JSON, YAML)
- Edit files in `theme/` directory (HTML templates, CSS, JavaScript)
- Edit files in `public/` directory if they are static assets (images, fonts, etc.)
- Create new files in allowed directories
- Delete files in allowed directories

## Forbidden Operations

- DO NOT edit or create hidden files (files starting with `.`)
- DO NOT edit binary files
- DO NOT create or edit server-side code (PHP, Python, Ruby, etc.)
- DO NOT modify configuration files outside the allowed directories
- DO NOT execute shell commands that could compromise the system
- DO NOT access network resources

## Required Output

After making changes, provide:

1. A summary of what you changed (2-3 sentences)
2. A list of files you modified, created, or deleted

## File Change Format

Please end your response with:

```
FILES_CHANGED:
- modified: path/to/file1
- created: path/to/file2
- deleted: path/to/file3
```

And:

```
SUMMARY:
Your 2-3 sentence summary here
```
