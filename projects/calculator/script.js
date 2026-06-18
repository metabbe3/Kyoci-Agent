let displayValue = '0';
let firstOperand = null;
let operator = null;

// Assuming there is a display element in index.html with id="display"
const display = document.getElementById('display');

function updateDisplay(value) {
    if (display) {
        display.value = value; // Changed from innerText to value for input tag
    }
}

function handleInput(input) {
    if (input === 'AC') { // All Clear
        displayValue = '0';
    } else if (input === 'C') { // Clear entry
        displayValue = '0';
    } else if (input === '=') {
        if (operator && firstOperand !== null) {
            const secondOperand = parseFloat(displayValue);
            let result;

            switch (operator) {
                case '+':
                    result = firstOperand + secondOperand;
                    break;
                case '-':
                    result = firstOperand - secondOperand;
                    break;
                case '*':
                    result = firstOperand * secondOperand;
                    break;
                case '/':
                    if (secondOperand !== 0) {
                        result = firstOperand / secondOperand;
                    } else {
                        displayValue = 'Error';
                        return;
                    }
                    break;
                default:
                    return;
            }

            displayValue = `${parseFloat(result.toFixed(7))}`;
            firstOperand = null;
            operator = null;
        }
    } else {
        // Handle number and decimal inputs
        if (displayValue === '0' && input !== '.') {
            displayValue = input;
        } else {
            displayValue += input;
        }
    }
    updateDisplay(displayValue);
}

function handleOperator(nextOperator) {
    const inputValue = parseFloat(displayValue);

    if (firstOperand === null) {
        firstOperand = inputValue;
    } else if (operator) {
        // Calculate intermediate result
        const secondOperand = parseFloat(displayValue);
        let result;

        switch (operator) {
            case '+':
                result = firstOperand + secondOperand;
                break;
            case '-':
                result = firstOperand - secondOperand;
                break;
            case '*':
                result = firstOperand * secondOperand;
                break;
            case '/':
                if (secondOperand !== 0) {
                    result = firstOperand / secondOperand;
                } else {
                    displayValue = 'Error';
                    return;
                }
                break;
            default:
                return;
        }

        displayValue = `${parseFloat(result.toFixed(7))}`;
        firstOperand = result;
    }

    operator = nextOperator;
    updateDisplay(displayValue);
}

document.addEventListener('DOMContentLoaded', () => {
    const buttons = document.querySelectorAll('.buttons button');

    buttons.forEach(button => {
        button.addEventListener('click', () => {
            const action = button.getAttribute('data-action');
            const number = button.getAttribute('data-number');

            if (number) {
                handleInput(number);
            } else if (action) {
                switch (action) {
                    case 'clear':
                        handleInput('AC'); // Using AC for full clear
                        break;
                    case 'backspace':
                        // Implement backspace logic: remove last character from displayValue
                        if (displayValue.length > 1) {
                            displayValue = displayValue.slice(0, -1);
                        } else {
                            displayValue = '0';
                        }
                        updateDisplay(displayValue);
                        break;
                    case 'decimal':
                        if (!displayValue.includes('.')) {
                            handleInput('.');
                        }
                        break;
                    case 'add':
                    case '-':
                    case '*':
                    case '/':
                        handleOperator(action);
                        break;
                    case 'calculate':
                        handleInput('=');
                        break;
                }
            }
        });
    });
});