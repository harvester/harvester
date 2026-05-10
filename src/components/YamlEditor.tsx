interface YamlEditorProps {
  value: string;
  onChange: (value: string) => void;
}

export function YamlEditor({ value, onChange }: YamlEditorProps) {
  return (
    <div className="yaml-editor">
      <div className="yaml-header">
        <h3>YAML editor</h3>
        <span className="note">Edit before applying</span>
      </div>
      <textarea value={value} onChange={(e) => onChange(e.target.value)} spellCheck={false} />
    </div>
  );
}
