//
// Copyright 2014 Cristian Maglie. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package serial

/*

// MSDN article on Serial Communications:
// http://msdn.microsoft.com/en-us/library/ff802693.aspx

// Arduino Playground article on serial communication with Windows API:
// http://playground.arduino.cc/Interfacing/CPPWindows

*/

import (
	"syscall"
	"time"
)

// opaque type that implements SerialPort interface for Windows
type SerialPort struct {
	handle syscall.Handle
}

//sys RegEnumValue(key syscall.Handle, index uint32, name *uint16, nameLen *uint32, reserved *uint32, class *uint16, value *uint16, valueLen *uint32) (regerrno error) = advapi32.RegEnumValueW

func GetPortsList() ([]string, error) {
	subKey, err := syscall.UTF16PtrFromString("HARDWARE\\DEVICEMAP\\SERIALCOMM\\")
	if err != nil {
		return nil, &SerialPortError{code: ERROR_ENUMERATING_PORTS}
	}

	var h syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE, subKey, 0, syscall.KEY_READ, &h) != nil {
		return nil, &SerialPortError{code: ERROR_ENUMERATING_PORTS}
	}
	defer syscall.RegCloseKey(h)

	var valuesCount uint32
	if syscall.RegQueryInfoKey(h, nil, nil, nil, nil, nil, nil, &valuesCount, nil, nil, nil, nil) != nil {
		return nil, &SerialPortError{code: ERROR_ENUMERATING_PORTS}
	}

	list := make([]string, valuesCount)
	for i := range list {
		var data [1024]uint16
		dataSize := uint32(len(data))
		var name [1024]uint16
		nameSize := uint32(len(name))
		if RegEnumValue(h, uint32(i), &name[0], &nameSize, nil, nil, &data[0], &dataSize) != nil {
			return nil, &SerialPortError{code: ERROR_ENUMERATING_PORTS}
		}
		list[i] = syscall.UTF16ToString(data[:])
	}
	return list, nil
}

func (port *SerialPort) Close() error {
	return syscall.CloseHandle(port.handle)
}

func (port *SerialPort) Read(p []byte) (int, error) {
	var readed uint32
	params := &DCB{}
	for {
		if err := syscall.ReadFile(port.handle, p, &readed, nil); err != nil {
			return int(readed), err
		}
		if readed > 0 {
			return int(readed), nil
		}

		// At the moment it seems that the only reliable way to check if
		// a serial port is alive in Windows is to check if the SetCommState
		// function fails.

		GetCommState(port.handle, params)
		if err := SetCommState(port.handle, params); err != nil {
			port.Close()
			return 0, err
		}
	}
}

func (port *SerialPort) Write(p []byte) (int, error) {
	var writed uint32
	err := syscall.WriteFile(port.handle, p, &writed, nil)
	return int(writed), err
}

func (port *SerialPort) SendBreak(breakTime int) error {
	// Set the Break
	if SetCommBreak(port.handle) != nil {
		port.Close()
		return &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}

	// Sleep for a period
	time.Sleep(time.Duration(breakTime) * time.Millisecond)

	// Reset the BREAK
	if ClearCommBreak(port.handle) != nil {
		port.Close()
		return &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}
	return nil
}

const (
	DCB_BINARY                   = 0x00000001
	DCB_PARITY                   = 0x00000002
	DCB_OUT_X_CTS_FLOW           = 0x00000004
	DCB_OUT_X_DSR_FLOW           = 0x00000008
	DCB_DTR_CONTROL_DISABLE_MASK = ^0x00000030
	DCB_DTR_CONTROL_ENABLE       = 0x00000010
	DCB_DTR_CONTROL_HANDSHAKE    = 0x00000020
	DCB_DSR_SENSITIVITY          = 0x00000040
	DCB_TX_CONTINUE_ON_XOFF      = 0x00000080
	DCB_OUT_X                    = 0x00000100
	DCB_IN_X                     = 0x00000200
	DCB_ERROR_CHAR               = 0x00000400
	DCB_NULL                     = 0x00000800
	DCB_RTS_CONTROL_DISABLE_MASK = ^0x00003000
	DCB_RTS_CONTROL_ENABLE       = 0x00001000
	DCB_RTS_CONTROL_HANDSHAKE    = 0x00002000
	DCB_RTS_CONTROL_TOGGLE       = 0x00003000
	DCB_ABORT_ON_ERROR           = 0x00004000
)

type DCB struct {
	DCBlength uint32
	BaudRate  uint32

	// Flags field is a bitfield
	//  fBinary            :1
	//  fParity            :1
	//  fOutxCtsFlow       :1
	//  fOutxDsrFlow       :1
	//  fDtrControl        :2
	//  fDsrSensitivity    :1
	//  fTXContinueOnXoff  :1
	//  fOutX              :1
	//  fInX               :1
	//  fErrorChar         :1
	//  fNull              :1
	//  fRtsControl        :2
	//  fAbortOnError      :1
	//  fDummy2            :17
	Flags uint32

	wReserved  uint16
	XonLim     uint16
	XoffLim    uint16
	ByteSize   byte
	Parity     byte
	StopBits   byte
	XonChar    byte
	XoffChar   byte
	ErrorChar  byte
	EofChar    byte
	EvtChar    byte
	wReserved1 uint16
}

type COMMTIMEOUTS struct {
	ReadIntervalTimeout         uint32
	ReadTotalTimeoutMultiplier  uint32
	ReadTotalTimeoutConstant    uint32
	WriteTotalTimeoutMultiplier uint32
	WriteTotalTimeoutConstant   uint32
}

//sys GetCommState(handle syscall.Handle, dcb *DCB) (err error)
//sys SetCommState(handle syscall.Handle, dcb *DCB) (err error)
//sys SetCommTimeouts(handle syscall.Handle, timeouts *COMMTIMEOUTS) (err error)

const (
	NOPARITY    = 0 // Default
	ODDPARITY   = 1
	EVENPARITY  = 2
	MARKPARITY  = 3
	SPACEPARITY = 4
)

const (
	ONESTOPBIT   = 0 // Default
	ONE5STOPBITS = 1
	TWOSTOPBITS  = 2
)

/// Set the Baud rate, data bits, stop bit and Parity
/// Default is 9600 8N1
func (port *SerialPort) SetMode(mode *Mode) error {
	params := DCB{}
	if GetCommState(port.handle, &params) != nil {
		port.Close()
		return &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}
	if mode.BaudRate == 0 {
		params.BaudRate = 9600 // Default to 9600
	} else {
		params.BaudRate = uint32(mode.BaudRate)
	}
	if mode.DataBits == 0 {
		params.ByteSize = 8 // Default to 8 bits
	} else {
		params.ByteSize = byte(mode.DataBits)
	}
	params.StopBits = byte(mode.StopBits)
	params.Parity = byte(mode.Parity)
	if SetCommState(port.handle, &params) != nil {
		port.Close()
		return &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}
	return nil
}

func OpenPort(portName string, mode *Mode) (*SerialPort, error) {
	portName = "\\\\.\\" + portName
	path, err := syscall.UTF16PtrFromString(portName)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(
		path,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, nil,
		syscall.OPEN_EXISTING,
		0, //syscall.FILE_FLAG_OVERLAPPED,
		0)
	if err != nil {
		switch err {
		case syscall.ERROR_ACCESS_DENIED:
			return nil, &SerialPortError{code: ERROR_PORT_BUSY}
		case syscall.ERROR_FILE_NOT_FOUND:
			return nil, &SerialPortError{code: ERROR_PORT_NOT_FOUND}
		}
		return nil, err
	}
	// Create the serial port
	port := &SerialPort{
		handle: handle,
	}

	// Set port parameters
	if port.SetMode(mode) != nil {
		port.Close()
		return nil, &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}

	params := &DCB{}
	if GetCommState(port.handle, params) != nil {
		port.Close()
		return nil, &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}
	params.Flags |= DCB_RTS_CONTROL_ENABLE | DCB_DTR_CONTROL_ENABLE
	params.Flags &= ^uint32(DCB_OUT_X_CTS_FLOW)
	params.Flags &= ^uint32(DCB_OUT_X_DSR_FLOW)
	params.Flags &= ^uint32(DCB_DSR_SENSITIVITY)
	params.Flags |= DCB_TX_CONTINUE_ON_XOFF
	params.Flags &= ^uint32(DCB_IN_X | DCB_OUT_X)
	params.Flags &= ^uint32(DCB_ERROR_CHAR)
	params.Flags &= ^uint32(DCB_NULL)
	params.Flags &= ^uint32(DCB_ABORT_ON_ERROR)
	params.XonLim = 2048
	params.XoffLim = 512
	params.XonChar = 17  // DC1
	params.XoffChar = 19 // C3
	if SetCommState(port.handle, params) != nil {
		port.Close()
		return nil, &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}

	// Set timeouts to 1 second
	timeouts := &COMMTIMEOUTS{
		ReadIntervalTimeout:         0xFFFFFFFF,
		ReadTotalTimeoutMultiplier:  0xFFFFFFFF,
		ReadTotalTimeoutConstant:    1000, // 1 sec
		WriteTotalTimeoutConstant:   0,
		WriteTotalTimeoutMultiplier: 0,
	}
	if SetCommTimeouts(port.handle, timeouts) != nil {
		port.Close()
		return nil, &SerialPortError{code: ERROR_INVALID_SERIAL_PORT}
	}

	return port, nil
}

// vi:ts=2
