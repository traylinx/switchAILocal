#!/usr/bin/env python3
"""
Quick validation script for skills - validates structure and content quality
"""

import sys
import re
from pathlib import Path

try:
    import yaml
except ImportError:
    print("Error: PyYAML not installed. Run: pip install pyyaml")
    sys.exit(1)


# Valid capability values
VALID_CAPABILITIES = {
    'coding', 'reasoning', 'creative', 'fast', 'secure', 
    'vision', 'audio', 'cli', 'long_ctx'
}

# Allowed frontmatter properties
ALLOWED_PROPERTIES = {'name', 'description', 'required-capability', 'allowed-tools', 'metadata'}


def validate_skill(skill_path):
    """
    Validate a skill directory.
    
    Returns:
        tuple: (is_valid: bool, message: str)
    """
    skill_path = Path(skill_path)
    warnings = []

    # Check directory exists
    if not skill_path.exists():
        return False, f"Skill directory not found: {skill_path}"
    
    if not skill_path.is_dir():
        return False, f"Path is not a directory: {skill_path}"

    # Check SKILL.md exists
    skill_md = skill_path / 'SKILL.md'
    if not skill_md.exists():
        return False, "SKILL.md not found"

    # Read content
    try:
        content = skill_md.read_text(encoding='utf-8')
    except Exception as e:
        return False, f"Failed to read SKILL.md: {e}"

    # Check frontmatter exists
    if not content.startswith('---'):
        return False, "No YAML frontmatter found (must start with ---)"

    # Extract frontmatter
    match = re.match(r'^---\n(.*?)\n---', content, re.DOTALL)
    if not match:
        return False, "Invalid frontmatter format (missing closing ---)"

    frontmatter_text = match.group(1)
    body = content[match.end():].strip()

    # Parse YAML frontmatter
    try:
        frontmatter = yaml.safe_load(frontmatter_text)
        if not isinstance(frontmatter, dict):
            return False, "Frontmatter must be a YAML dictionary"
    except yaml.YAMLError as e:
        return False, f"Invalid YAML in frontmatter: {e}"

    # Check for unexpected properties
    unexpected_keys = set(frontmatter.keys()) - ALLOWED_PROPERTIES
    if unexpected_keys:
        return False, (
            f"Unexpected key(s) in frontmatter: {', '.join(sorted(unexpected_keys))}. "
            f"Allowed: {', '.join(sorted(ALLOWED_PROPERTIES))}"
        )

    # Validate 'name' field
    if 'name' not in frontmatter:
        return False, "Missing 'name' in frontmatter"
    
    name = frontmatter.get('name', '')
    if not isinstance(name, str):
        return False, f"Name must be a string, got {type(name).__name__}"
    
    name = name.strip()
    if not name:
        return False, "Name cannot be empty"
    
    # Check naming convention (kebab-case)
    if not re.match(r'^[a-z0-9]+(-[a-z0-9]+)*$', name):
        return False, f"Name '{name}' must be kebab-case (lowercase letters, digits, hyphens)"
    
    if len(name) > 64:
        return False, f"Name too long ({len(name)} chars). Maximum is 64."

    # Check name matches directory
    if name != skill_path.name:
        warnings.append(f"Name '{name}' doesn't match directory '{skill_path.name}'")

    # Validate 'description' field
    if 'description' not in frontmatter:
        return False, "Missing 'description' in frontmatter"
    
    description = frontmatter.get('description', '')
    if not isinstance(description, str):
        return False, f"Description must be a string, got {type(description).__name__}"
    
    description = description.strip()
    if not description:
        return False, "Description cannot be empty"
    
    if len(description) < 20:
        warnings.append("Description is very short. Consider being more descriptive.")
    
    if len(description) > 1024:
        return False, f"Description too long ({len(description)} chars). Maximum is 1024."
    
    if '<' in description or '>' in description:
        return False, "Description cannot contain angle brackets (< or >)"

    # Check for TODO placeholders
    if '[TODO' in description or 'TODO:' in description:
        return False, "Description contains TODO placeholder - please complete it"

    # Validate 'required-capability' if present
    capability = frontmatter.get('required-capability')
    if capability is not None:
        if not isinstance(capability, str):
            return False, f"required-capability must be a string, got {type(capability).__name__}"
        if capability not in VALID_CAPABILITIES:
            return False, (
                f"Invalid required-capability '{capability}'. "
                f"Valid options: {', '.join(sorted(VALID_CAPABILITIES))}"
            )

    # Validate body content
    if not body:
        return False, "SKILL.md body is empty"
    
    if len(body) < 50:
        warnings.append("SKILL.md body is very short. Consider adding more guidance.")
    
    # Check for TODO placeholders in body
    if '[TODO' in body:
        warnings.append("Body contains [TODO] placeholders - consider completing them")

    # Check body length (warn if too long)
    body_lines = body.count('\n') + 1
    if body_lines > 500:
        warnings.append(f"SKILL.md is {body_lines} lines. Consider splitting into references/")

    # Check for unnecessary files
    unnecessary_files = ['README.md', 'CHANGELOG.md', 'INSTALLATION.md', 'QUICK_REFERENCE.md']
    for filename in unnecessary_files:
        if (skill_path / filename).exists():
            warnings.append(f"Unnecessary file found: {filename}")

    # Check resource directories
    scripts_dir = skill_path / 'scripts'
    references_dir = skill_path / 'references'
    assets_dir = skill_path / 'assets'

    # Warn about empty directories
    for dir_path, dir_name in [(scripts_dir, 'scripts'), (references_dir, 'references'), (assets_dir, 'assets')]:
        if dir_path.exists() and dir_path.is_dir():
            files = list(dir_path.iterdir())
            if not files:
                warnings.append(f"Empty directory: {dir_name}/ - consider removing if not needed")

    # Build result message
    if warnings:
        warning_text = "\n  - ".join(warnings)
        return True, f"Skill is valid with warnings:\n  - {warning_text}"
    
    return True, "Skill is valid!"


def main():
    if len(sys.argv) != 2:
        print("Usage: python quick_validate.py <skill_directory>")
        print("\nValidates a skill directory for:")
        print("  - SKILL.md presence and format")
        print("  - Frontmatter structure (name, description, required-capability)")
        print("  - Naming conventions (kebab-case)")
        print("  - Content quality (no TODOs, reasonable length)")
        print("  - Unnecessary files")
        sys.exit(1)
    
    skill_path = sys.argv[1]
    print(f"Validating: {skill_path}\n")
    
    valid, message = validate_skill(skill_path)
    
    if valid:
        print(f"✅ {message}")
    else:
        print(f"❌ {message}")
    
    sys.exit(0 if valid else 1)


if __name__ == "__main__":
    main()
