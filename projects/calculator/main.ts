import { CalculatorEngine } from './CalculatorEngine';

// Assume a global or imported UI element representing the calculator display and buttons
const display = document.getElementById('display');
const buttons = document.querySelectorAll('.calc-button');

// Initialize the calculation engine
const calculator = new CalculatorEngine();

/**
 * Handles button clicks and updates the display.
 * @param {string} value - The value pressed on the button.
 */
function handleButtonClick(value) {
    if (typeof value === 'string') {
        // Handle number/operator input
        calculator.input(value);
        display.textContent += value;
    } else if (typeof value === 'number') {
        // Handle actions like equals or clear
        if (value === 'equals') {
            const result = calculator.calculate();
            display.textContent = result;
        } else if (value === 'clear') {
            calculator.clear();
            display.textContent = '';
        }
    }
}

// Attach event listeners to all calculator buttons
buttons.forEach(button => {
    button.addEventListener('click', () => {
        const value = button.textContent; // Assuming textContent holds the operation/number
        handleButtonClick(value);
    });
});

// Initial setup or binding logic goes here...
console.log("Calculator application initialized.");