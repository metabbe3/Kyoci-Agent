import { jsx as _jsx } from "react/jsx-runtime";
import ReactDOM from 'react-dom';
function App() {
    return (_jsx("div", { children: _jsx("h1", { children: "Calculator" }) }));
}
ReactDOM.render(_jsx(App, {}), document.getElementById('root'));
