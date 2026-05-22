import CodeMirror from '@uiw/react-codemirror';
import { yaml } from '@codemirror/lang-yaml';
import { oneDark } from '@codemirror/theme-one-dark';

interface YamlEditorProps {
  value: string;
  onChange: (value: string) => void;
  validationIssues?: string[];
}

export function YamlEditor({ value, onChange, validationIssues = [] }: YamlEditorProps) {
  return (
    <div className="yaml-editor">
      <div className="yaml-header">
        <h3>CodeMirror YAML editor</h3>
        <span className={validationIssues.length > 0 ? 'note note-warning' : 'note'}>{validationIssues.length > 0 ? `${validationIssues.length} validation notes` : 'Validation clean'}</span>
      </div>
      <CodeMirror
        value={value}
        height="520px"
        theme={oneDark}
        extensions={[yaml()]}
        basicSetup={{
          autocompletion: true,
          bracketMatching: true,
          foldGutter: true,
          highlightActiveLine: true,
          lineNumbers: true,
        }}
        onChange={onChange}
      />
      {validationIssues.length > 0 && (
        <ul className="yaml-validation-list">
          {validationIssues.map((issue) => (
            <li key={issue}>{issue}</li>
          ))}
        </ul>
      )}
    </div>
  );
}
