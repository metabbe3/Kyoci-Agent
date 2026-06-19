"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || (function () {
    var ownKeys = function(o) {
        ownKeys = Object.getOwnPropertyNames || function (o) {
            var ar = [];
            for (var k in o) if (Object.prototype.hasOwnProperty.call(o, k)) ar[ar.length] = k;
            return ar;
        };
        return ownKeys(o);
    };
    return function (mod) {
        if (mod && mod.__esModule) return mod;
        var result = {};
        if (mod != null) for (var k = ownKeys(mod), i = 0; i < k.length; i++) if (k[i] !== "default") __createBinding(result, mod, k[i]);
        __setModuleDefault(result, mod);
        return result;
    };
})();
Object.defineProperty(exports, "__esModule", { value: true });
const react_1 = __importStar(require("react"));
const initialState = { display: '0' };
const App = () => {
    const [state, setState] = (0, react_1.useState)(initialState);
    const handleDigitClick = (digit) => {
        setState(prevState => ({
            display: prevState.display === '0' ? digit : prevState.display + digit,
        }));
    };
    const handleClearClick = () => {
        setState(initialState);
    };
    return (<div className="calculator">
      <input type="text" value={state.display} readOnly/>
      <button onClick={() => handleDigitClick('7')}>7</button>
      <button onClick={() => handleDigitClick('8')}>8</button>
      <button onClick={() => handleDigitClick('9')}>9</button>
      <button onClick={handleClearClick}>C</button>
      <button onClick={() => handleDigitClick('4')}>4</button>
      <button onClick={() => handleDigitClick('5')}>5</button>
      <button onClick={() => handleDigitClick('6')}>6</button>
      <button onClick={() => handleDigitClick('1')}>1</button>
      <button onClick={() => handleDigitClick('2')}>2</button>
      <button onClick={() => handleDigitClick('3')}>3</button>
      <button onClick={() => handleDigitClick('0')}>0</button>
      <button onClick={() => handleDigitClick('.')}>.</button>
      <button>=</button>
    </div>);
};
exports.default = App;
