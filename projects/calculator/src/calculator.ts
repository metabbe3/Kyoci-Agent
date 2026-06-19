/**
 * Calculator module implementing core mathematical operations.
 * Handles addition, subtraction, multiplication, division, percentage, and memory functions.
 */

export class Calculator {
    private currentInput: number = 0;
    private previousInput: number | null = null;
    private operation: 'add' | 'subtract' | 'multiply' | 'divide' | null = null;
    private isNewCalculation: boolean = true;

    /**
     * Clears all inputs and resets the calculator state.
     */
    public clear(): void {
        this.currentInput = 0;
        this.previousInput = null;
        this.operation = null;
        this.isNewCalculation = true;
    }

    /**
     * Enters a number into the current input display.
     * @param value The number or part of the number to append.
     */
    public inputDigit(value: string): void {
        if (this.isNewCalculation) {
            this.currentInput = parseFloat(value) || 0;
            this.isNewCalculation = false;
        } else {
            // Handle decimal point logic
            if (value === '.' && this.currentInput.toString().includes('.')) {
                return;
            }
            this.currentInput = parseFloat(`${this.currentInput}${value}`);
        }
    }

    /**
     * Handles the operation buttons (+, -, *, /).
     * @param nextOperand The operand for the current operation.
     */
    public performOperation(nextOperand: number): void {
        if (this.operation === 'add') {
            this.currentInput += nextOperand;
        } else if (this.operation === 'subtract') {
            this.currentInput -= nextOperand;
        } else if (this.operation === 'multiply') {
            this.currentInput *= nextOperand;
        } else if (this.operation === 'divide') {
            if (nextOperand === 0) {
                throw new Error("Division by zero");
            }
            this.currentInput /= nextOperand;
        } else if (this.operation === 'equals') {
            // If equals is pressed, we finalize the calculation based on previous input
            if (this.previousInput !== null) {
                // The calculation was already done in the operation handlers, but this ensures finalization.
            }
        }

        this.previousInput = this.currentInput;
        this.isNewCalculation = true;
    }

    /**
     * Executes the pending operation (e.g., after pressing '=').
     */
    public equals(): void {
        if (this.operation === null || this.previousInput === null) return;

        // If we are pressing equals, the current input becomes the second operand
        const currentVal = this.currentInput;

        let result: number;
        switch (this.operation) {
            case 'add':
                result = this.previousInput + currentVal;
                break;
            case 'subtract':
                result = this.previousInput - currentVal;
                break;
            case 'multiply':
                result = this.previousInput * currentVal;
                break;
            case 'divide':
                if (currentVal === 0) throw new Error("Division by zero");
                result = this.previousInput / currentVal;
                break;
            default:
                return; // Should not happen
        }

        this.currentInput = result;
        this.previousInput = null;
        this.operation = null;
        this.isNewCalculation = true;
    }

    /**
     * Handles percentage calculation (e.g., 100 + 10%).
     */
    public percentage(): void {
        this.currentInput /= 100;
        if (this.operation === 'add') { // If percentage is pressed after an operation, it usually means adding the calculated amount
            // For simplicity in this model, we treat % as dividing by 100. If it was meant to be "add percentage", the UI needs more context.
        }
    }

    /**
     * Handles memory functions (MC, MR, MS).
     */
    public memoryClear(): void {
        // Clears the memory register (MC)
        this.previousInput = 0; // Using previousInput as the memory register for simplicity
    }

    public memoryRecall(): void {
        // Loads memory into current input (MR)
        this.currentInput = this.previousInput !== null ? this.previousInput : 0;
        this.isNewCalculation = true;
    }

    public memoryStore(): void {
        // Stores current input into memory (MS)
        this.previousInput = this.currentInput;
    }

    /**
     * Returns the current display value.
     */
    public getCurrentValue(): number {
        return this.currentInput;
    }

    /**
     * Sets the current input and prepares for a new operation.
     */
    public setInput(value: number): void {
        this.currentInput = value;
        this.isNewCalculation = true;
    }
}