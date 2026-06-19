"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
const react_1 = require("react");
const useCalculator = () => {
    const [state, setState] = (0, react_1.useState)({ current: '0', previous: '', operation: null });
    const handleNumber = (number) => {
        // Implementation for handling number button press
    };
    const handleOperation = (operation) => {
        // Implementation for handling operation button press
    };
    const handleEquals = () => {
        // Implementation for handling equals button press
    };
    const handleClear = () => {
        // Implementation for handling clear button press
    };
    return {
        current: state.current,
        handleNumber,
        handleOperation,
        handleEquals,
        handleClear,
    };
};
exports.default = useCalculator;
