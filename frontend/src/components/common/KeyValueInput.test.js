import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { KeyValueInput } from './KeyValueInput';

describe('KeyValueInput', () => {
    const mockOnChange = jest.fn();

    beforeEach(() => {
        mockOnChange.mockClear();
    });

    test('renders with label', () => {
        render(<KeyValueInput label="Headers" onChange={mockOnChange} />);
        expect(screen.getByText('Headers')).toBeInTheDocument();
    });

    test('renders initial empty pair if no pairs provided', () => {
        render(<KeyValueInput label="Headers" onChange={mockOnChange} />);
        expect(screen.getAllByPlaceholderText('Key')).toHaveLength(1);
        expect(screen.getAllByPlaceholderText('Value')).toHaveLength(1);
    });

    test('renders provided pairs', () => {
        const pairs = { 'Content-Type': 'application/json' };
        render(<KeyValueInput label="Headers" pairs={pairs} onChange={mockOnChange} />);
        
        expect(screen.getByDisplayValue('Content-Type')).toBeInTheDocument();
        expect(screen.getByDisplayValue('application/json')).toBeInTheDocument();
    });

    test('calls onChange when values change', async () => {
        const user = userEvent.setup();
        render(<KeyValueInput label="Headers" onChange={mockOnChange} />);
        
        const keyInput = screen.getAllByPlaceholderText('Key')[0];
        const valueInput = screen.getAllByPlaceholderText('Value')[0];

        await user.type(keyInput, 'Authorization');
        await user.type(valueInput, 'Bearer token');

        expect(mockOnChange).toHaveBeenCalled();
        expect(mockOnChange).toHaveBeenLastCalledWith({ 'Authorization': 'Bearer token' });
    });

    test('adds new pair when add button clicked', async () => {
        const user = userEvent.setup();
        render(<KeyValueInput label="Headers" onChange={mockOnChange} />);
        
        const addButton = screen.getByText('Add Header');
        await user.click(addButton);

        expect(screen.getAllByPlaceholderText('Key')).toHaveLength(2);
    });

    test('removes pair when remove button clicked', async () => {
        const user = userEvent.setup();
        const pairs = { 'A': '1', 'B': '2' };
        render(<KeyValueInput label="Headers" pairs={pairs} onChange={mockOnChange} />);
        
        const removeButtons = screen.getAllByTitle('Remove header');
        expect(removeButtons).toHaveLength(2);
        
        // Remove first item
        await user.click(removeButtons[0]);
        
        // Should trigger onChange with remaining pair
        expect(mockOnChange).toHaveBeenLastCalledWith({ 'B': '2' });
        
        // UI should update (though strictly we rely on parent to update props usually, 
        // internal state handles optimistic UI update)
        expect(screen.queryByDisplayValue('A')).not.toBeInTheDocument();
    });
});
