import MinecraftProperties from 'components/editor/languages/minecraft-properties'

export default function loadCustomEditorSettings() {
    // Register languages
    MinecraftProperties()
}

export const LanguageOptions = [
    { value: 'plaintext', label: 'Plain Text' },
    { value: 'json', label: 'JSON' },
    { value: 'html', label: 'HTML' },
    { value: 'xml', label: 'XML' },
    { value: 'sql', label: 'SQL' },
    { value: 'css', label: 'CSS' },
    { value: 'scss', label: 'SCSS' },
    { value: 'less', label: 'LESS' },
    { value: 'javascript', label: 'JavaScript' },
    { value: 'typescript', label: 'TypeScript' },
    { value: 'csharp', label: 'C#' },
    { value: 'python', label: 'Python' },
    { value: 'java', label: 'Java' },
    { value: 'php', label: 'PHP' },
    { value: 'ruby', label: 'Ruby' },
    { value: 'go', label: 'Go' },
    { value: 'c', label: 'C' },
    { value: 'cpp', label: 'C++' },
    { value: 'rust', label: 'Rust' },
    { value: 'perl', label: 'Perl' },
    { value: 'bash', label: 'Bash' },
    { value: 'markdown', label: 'Markdown' },
    { value: 'yaml', label: 'YAML' },
    { value: 'properties', label: 'Properties' },
    { value: 'minecraft-properties', label: 'Minecraft Config' },
]

export function getLanguageFromFileName(fileName: string): string {
    if (fileName === 'server.properties') return 'minecraft-properties'
    switch (fileName.split('.').pop()) {
        case 'json':
            return 'json'
        case 'html':
            return 'html'
        case 'xml':
            return 'xml'
        case 'sql':
            return 'sql'
        case 'css':
            return 'css'
        case 'scss':
            return 'scss'
        case 'less':
            return 'less'
        case 'javascript':
            return 'javascript'
        case 'typescript':
            return 'typescript'
        case 'csharp':
            return 'csharp'
        case 'python':
            return 'python'
        case 'java':
            return 'java'
        case 'php':
            return 'php'
        case 'ruby':
            return 'ruby'
        case 'go':
            return 'go'
        case 'c':
            return 'c'
        case 'cpp':
            return 'cpp'
        case 'rust':
            return 'rust'
        case 'perl':
            return 'perl'
        case 'bash':
            return 'bash'
        case 'markdown':
            return 'markdown'
        case 'yaml':
            return 'yaml'
        case 'properties':
            return 'ini'
        case 'minecraft-properties':
            return 'minecraft-properties'
        default:
            return 'plaintext'
    }
}
