// Global variables to hold the state of the calculator
let currentOperand = '0';
let previousOperand = null;
let operation = null;

// DOM elements references (assuming they are available globally or passed in)
const displayCurrentOperand = document.getElementById('current-operation');
const displayResult = document.getElementById('result');

/**
 * Updates the displayed values on the screen.
 */
function updateDisplay() {
    displayCurrentOperand.textContent = currentOperand;
    displayResult.textContent = previousOperand ? `${currentOperand} ${operation}` : currentOperand;
}

/**
 * Handles number button clicks.
 * @param {string} num - The number pressed.
 */
function appendNumber(num) {
    if (currentOperand === '0' && num !== '.') {
        currentOperand = num;
    } else {
        currentOperand += num;
    }
}

/**
 * Handles decimal point clicks.
 */
function appendDecimal() {
    if (!currentOperand.includes('.')) {
        currentOperand += '.';
    }
}

/**
 * Clears all operands and operations.
 */
function clear() {
    currentOperand = '0';
    previousOperand = null;
    operation = null;
}

/**
 * Handles backspace/delete clicks.
 */
function backspace() {
    if (currentOperand.length > 1) {
        currentOperand = currentOperand.slice(0, -1);
    } else {
        currentOperand = '0';
    }
}

/**
 * Sets the current operand as the previous and applies a new operation.
 * @param {string} nextOperation - The mathematical operation (+, -, *, /).
 */
function performOperation(nextOperation) {
    // If there is a previous operation waiting, calculate the intermediate result first
    if (previousOperand !== null) {
        calculate(); // Calculate the pending operation before starting a new one
    }

    // Set up for the new operation
    operation = nextOperation;
    previousOperand = currentOperand; // Current becomes the previous operand
    // Note: The current number is implicitly waiting for the next input or equals press
}

/**
 * Performs the calculation based on the stored operation.
 */
function calculate() {
    const prev = parseFloat(previousOperand);
    const current = parseFloat(currentOperand);

    if (isNaN(prev) || isNaN(current)) return; // Safety check

    let result = 0;

    try {
        switch (operation) {
            case '+':
                result = prev + current;
                break;
            case '-':
                result = prev - current;
                break;
            case '*':
                result = prev * current;
                break;
            case '/':
                if (current === 0) {
                    result = 'Error'; // Handle division by zero gracefully
                } else {
                    result = prev / current;
                }
                break;
            default:
                return;
        }

        // Update the current operand with the result and reset previous state
        currentOperand = String(parseFloat(result.toFixed(10))); // Limit floating point errors
        previousOperand = null;
        operation = null;

    } catch (e) {
        currentOperand = 'Error';
        previousOperand = null;
        operation = null;
    }
}

// --- Event Listeners Setup (Simulated binding based on HTML structure) ---
document.querySelectorAll('.btn').forEach(button => {
    button.addEventListener('click', () => {
        const action = button.getAttribute('data-action');

        if (action === 'number') {
            const num = button.textContent;
            appendNumber(num);
        } else if (action === 'decimal') {
            appendDecimal();
        } else if (action === 'clear') {
            clear();
        } else if (action === 'backspace') {
            backspace();
        } else if (action === 'add' || action === 'subtract' || action === 'multiply' || action === 'divide') {
            performOperation(action);
        } else if (action === 'calculate') {
            calculate();
        }

        updateDisplay(); // Always update after any action
    });
});


// Initial display setup (optional, but good practice)
updateDisplay();