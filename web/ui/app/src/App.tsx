import CodeMirror, { EditorView } from "@uiw/react-codemirror";
import { PromQLExtension } from "@prometheus-io/codemirror-promql";

const promqlExtension = new PromQLExtension();
function App() {

  return (
      <CodeMirror
          basicSetup={false}
          value="rate(foo)"
          editable={false}
          extensions={[
              promqlExtension.asExtension(),
              EditorView.lineWrapping,
          ]}
      />
  )
}

export default App
