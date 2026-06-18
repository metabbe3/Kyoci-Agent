/**
 * CalculatorEngine.ts
 * Core logic for the calculator application.
 */

export class CalculatorEngine {
    private currentInput: number = 0;
    private pendingOperation: string | null = null;

    /**
     * Sets the current display value.
     * @param value The number to be displayed/used as the starting point for calculation.
     */
    public clear() {
        this.currentInput = 0;
        this.pendingOperation = null;
    }

    /**
     * Handles number input.
     * @param digit The numeric value pressed.
     */
    public appendDigit(digit: string): void {
        if (this.pendingOperation !== null) {
            // If an operation is pending, pressing a digit starts the next operand.
            this.currentInput = parseFloat(digit) || 0;
        } else {
            // Otherwise, append to the current number.
            const currentValue = this.currentInput;
            if (digit === '.') {
                // Simple logic to prevent multiple decimals
                if (!String(currentValue).includes('.')) {
                    this.currentInput = `${currentValue}.${digit}`;
                }
            } else {
                this.currentInput = (currentValue === 0 && digit !== '.') ? parseFloat(digit) : `${currentValue}${digit}`;
            }
        }
    }

    /**
     * Sets the current input as the first operand and waits for a second operand.
     * @param operation The arithmetic operation (+, -, *, /).
     */
    public setOperation(operation: string): void {
        if (this.pendingOperation === null) {
            this.pendingOperation = operation;
        } else if (operation !== '=') {
            // If a new operation is pressed before the calculation, perform the previous one.
            this.calculate();
            this.pendingOperation = operation;
        }
    }

    /**
     * Performs the calculation based on the pending operation.
     * @returns The result of the calculation.
     */
    public calculate(): number {
        if (this.pendingOperation === null) return this.currentInput;

        const secondOperand = parseFloat(this.currentInput);
        let result: number;

        switch (this.pendingOperation) {
            case '+':
                result = this.currentInput + secondOperand;
                break;
            case '-':
                result = this.currentInput - secondOperand;
                break;
            case '*':
                result = this.currentInput * secondOperand;
                break;
            case '/':
                if (secondOperand === 0) {
                    throw new Error("Division by zero");
                }
                result = this.currentInput / secondOperand;
                break;
            default:
                throw new Error("Unknown operation");
        }

        // Update state for chaining operations
        this.currentInput = result;
        this.pendingOperation = null; // Operation is complete, waiting for next input/operation
        return result;
    }

    /**
     * Returns the current value displayed on the calculator.
     */
    public getCurrentValue(): number {
        return this.currentInput;
    }
}