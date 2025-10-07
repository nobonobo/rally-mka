package preset

import (
	"math"

	"github.com/mokiat/gomath/dprec"
	"github.com/mokiat/lacking/app"
	"github.com/mokiat/lacking/game/ecs"
	"github.com/mokiat/lacking/game/graphics"
	"github.com/mokiat/lacking/game/physics/collision"
	"github.com/mokiat/lacking/ui"
)

const (
	idleRPM   = 800.0  // アイドリング回転数
	maxRPM    = 6000.0 // 最大回転数
	respSpeed = 3000.0 // 回転数変化の速度（RPM/秒）
)

func NewCarSystem(ecsScene *ecs.Scene, gfxScene *graphics.Scene, gamepadProvider GamepadProvider, carDefinition *CarDefinition) *CarSystem {
	return &CarSystem{
		ecsScene:        ecsScene,
		gfxScene:        gfxScene,
		gamepadProvider: gamepadProvider,
		carDefinition:   carDefinition,
		torque:          MovingAverage(1),
		forceL:          MovingAverage(3),
		forceR:          MovingAverage(3),
		susL:            MovingAverage(6),
		susR:            MovingAverage(6),

		keysOfInterest: make(map[app.KeyCode]struct{}),
		keyStates:      make(map[app.KeyCode]bool),

		mouseOfInterest:   false,
		mouseButtonStates: make(map[app.MouseButton]bool),
		mouseAreaWidth:    1.0,
		mouseAreaHeight:   1.0,
	}
}

type CarSystem struct {
	ecsScene        *ecs.Scene
	gfxScene        *graphics.Scene
	gamepadProvider GamepadProvider
	carDefinition   *CarDefinition
	ffbTick         float64
	ffbForce        float64
	rpm             float64
	torque          func(float64) float64
	forceL          func(float64) float64
	forceR          func(float64) float64
	susL            func(float64) float64
	susR            func(float64) float64
	lastSteerAngle  dprec.Angle

	keysOfInterest map[ui.KeyCode]struct{}
	keyStates      map[ui.KeyCode]bool

	mouseOfInterest   bool
	mouseButtonStates map[ui.MouseButton]bool
	mouseAreaWidth    int
	mouseAreaHeight   int
	mouseX            int
	mouseY            int
	mouseScroll       float64
}

func (s *CarSystem) OnMouseEvent(element *ui.Element, event ui.MouseEvent) bool {
	if !s.mouseOfInterest {
		return false
	}
	bounds := element.Bounds()
	s.mouseAreaWidth = bounds.Width
	s.mouseAreaHeight = bounds.Height
	switch event.Type {
	case ui.MouseEventTypeDown:
		s.mouseButtonStates[event.Button] = true
	case ui.MouseEventTypeUp:
		s.mouseButtonStates[event.Button] = false
	case ui.MouseEventTypeScroll:
		s.mouseScroll += event.ScrollY
	case ui.MouseEventTypeMove:
		s.mouseX = event.Position.X
		s.mouseY = event.Position.Y
	}
	return true
}

func (s *CarSystem) OnKeyboardEvent(event ui.KeyboardEvent) bool {
	if _, ok := s.keysOfInterest[event.Code]; !ok {
		return false
	}
	switch event.Type {
	case ui.KeyboardEventTypeKeyDown:
		s.keyStates[event.Code] = true
	case ui.KeyboardEventTypeKeyUp:
		s.keyStates[event.Code] = false
	}
	return true
}

func (s *CarSystem) Update(elapsedSeconds float64) {
	s.mouseOfInterest = false

	result := s.ecsScene.Find(ecs.Having(CarComponentID))
	defer result.Close()

	var entity *ecs.Entity
	for result.FetchNext(&entity) {
		var keyboardControl *CarKeyboardControl
		if ecs.FetchComponent(entity, &keyboardControl) {
			s.updateKeyboard(elapsedSeconds, entity)
		}

		var mouseControl *CarMouseControl
		if ecs.FetchComponent(entity, &mouseControl) {
			s.mouseOfInterest = true
			s.updateMouse(elapsedSeconds, entity)
		}

		var gamepadControl *CarGamepadControl
		if ecs.FetchComponent(entity, &gamepadControl) {
			s.updateGamepad(elapsedSeconds, entity)
		}

		s.updateCar(elapsedSeconds, entity)
	}
}

func (s *CarSystem) updateRPM(throttle, elapsedSeconds float64) {
	// Throttleの範囲を0..1に制限
	if throttle < 0 {
		throttle = 0
	} else if throttle > 1 {
		throttle = 1
	}
	// 目標回転数はThrottleに比例してアイドリング〜最大回転数を線形補間
	targetRPM := idleRPM + throttle*(maxRPM-idleRPM)
	// 現在の回転数から目標回転数に向かってdelta秒間でrespSpeedを最大変化量として漸近的に変化させる
	diff := targetRPM - s.rpm
	// 回転数の増減量はrespSpeed * delta秒、現状との差分の符号付き量
	maxChange := respSpeed * elapsedSeconds
	// 差分を過度に変化させないようクランプ
	if math.Abs(diff) > maxChange {
		if diff > 0 {
			diff = maxChange
		} else {
			diff = -maxChange
		}
	}
	// 回転数を更新
	s.rpm += diff
	// 回転数の下限はアイドリング回転数だが、停止したいなら追加ロジック必要
	if s.rpm < idleRPM {
		s.rpm = idleRPM
	}
}

func (s *CarSystem) updateKeyboard(elapsedSeconds float64, entity *ecs.Entity) {
	var carComp *CarComponent
	ecs.FetchComponent(entity, &carComp)
	var keyboardComp *CarKeyboardControl
	ecs.FetchComponent(entity, &keyboardComp)

	s.keysOfInterest[keyboardComp.AccelerateKey] = struct{}{}
	s.keysOfInterest[keyboardComp.DecelerateKey] = struct{}{}
	s.keysOfInterest[keyboardComp.TurnLeftKey] = struct{}{}
	s.keysOfInterest[keyboardComp.TurnRightKey] = struct{}{}
	s.keysOfInterest[keyboardComp.ShiftUpKey] = struct{}{}
	s.keysOfInterest[keyboardComp.ShiftDownKey] = struct{}{}
	s.keysOfInterest[keyboardComp.RecoverKey] = struct{}{}

	if s.keyStates[keyboardComp.AccelerateKey] {
		carComp.Acceleration += elapsedSeconds * keyboardComp.AccelerationChangeSpeed
	} else {
		carComp.Acceleration -= elapsedSeconds * keyboardComp.AccelerationChangeSpeed
	}
	carComp.Acceleration = dprec.Clamp(carComp.Acceleration, 0.0, 1.0)

	if s.keyStates[keyboardComp.DecelerateKey] {
		carComp.Deceleration += elapsedSeconds * keyboardComp.DecelerationChangeSpeed
	} else {
		carComp.Deceleration -= elapsedSeconds * keyboardComp.DecelerationChangeSpeed
	}
	carComp.Deceleration = dprec.Clamp(carComp.Deceleration, 0.0, 1.0)

	autoMaxSteeringAmount := 1.0 / (1.0 + 0.05*carComp.Car.Velocity())
	switch {
	case s.keyStates[keyboardComp.TurnLeftKey] == s.keyStates[keyboardComp.TurnRightKey]:
		if keyboardComp.SteeringAmount > 0.0 {
			keyboardComp.SteeringAmount -= elapsedSeconds * keyboardComp.SteeringRestoreSpeed
			keyboardComp.SteeringAmount = dprec.Max(0.0, keyboardComp.SteeringAmount)
		}
		if keyboardComp.SteeringAmount < 0.0 {
			keyboardComp.SteeringAmount += elapsedSeconds * keyboardComp.SteeringRestoreSpeed
			keyboardComp.SteeringAmount = dprec.Min(0.0, keyboardComp.SteeringAmount)
		}
	case s.keyStates[keyboardComp.TurnLeftKey]:
		keyboardComp.SteeringAmount -= elapsedSeconds * keyboardComp.SteeringChangeSpeed
		keyboardComp.SteeringAmount = dprec.Max(keyboardComp.SteeringAmount, -autoMaxSteeringAmount)
	case s.keyStates[keyboardComp.TurnRightKey]:
		keyboardComp.SteeringAmount += elapsedSeconds * keyboardComp.SteeringChangeSpeed
		keyboardComp.SteeringAmount = dprec.Min(keyboardComp.SteeringAmount, autoMaxSteeringAmount)
	}

	maxSteeringAngle := carComp.Car.Axes()[0].MaxSteeringAngle()
	steeringAngle := maxSteeringAngle * dprec.Angle(keyboardComp.SteeringAmount)

	carDirection := carComp.Car.Chassis().Body().Orientation().OrientationZ()
	carDirection.Y = 0.0

	carActualDirection := carComp.Car.Chassis().Body().Velocity()
	carActualDirection.Y = 0.0

	recoverSin := dprec.Vec3Cross(
		dprec.UnitVec3(carDirection),
		dprec.UnitVec3(carActualDirection),
	)
	var recoverAngle dprec.Angle
	if (carActualDirection.Length() > 10.0) && (recoverSin.Length() > 0.000001) {
		recoverAngle = -dprec.Angle(dprec.Sign(recoverSin.Y)) * dprec.Asin(recoverSin.Length())
		recoverAngle = dprec.Clamp(recoverAngle, -maxSteeringAngle, maxSteeringAngle)
	}

	recoverAngle = recoverAngle / 1.5
	carComp.SteeringAmount = float64(dprec.Clamp(steeringAngle+recoverAngle, -maxSteeringAngle, maxSteeringAngle) / maxSteeringAngle)

	if s.keyStates[keyboardComp.ShiftDownKey] {
		carComp.Gear = CarGearReverse
	}
	if s.keyStates[keyboardComp.ShiftUpKey] {
		carComp.Gear = CarGearForward
	}
	carComp.Recover = s.keyStates[keyboardComp.RecoverKey]
}

func (s *CarSystem) updateMouse(elapsedSeconds float64, entity *ecs.Entity) {
	var carComp *CarComponent
	ecs.FetchComponent(entity, &carComp)
	var mouseComp *CarMouseControl
	ecs.FetchComponent(entity, &mouseComp)

	if s.mouseButtonStates[ui.MouseButtonLeft] {
		carComp.Acceleration += elapsedSeconds * mouseComp.AccelerationChangeSpeed
	} else {
		carComp.Acceleration -= elapsedSeconds * mouseComp.AccelerationChangeSpeed
	}
	carComp.Acceleration = dprec.Clamp(carComp.Acceleration, 0.0, 1.0)

	if s.mouseButtonStates[ui.MouseButtonRight] {
		carComp.Deceleration += elapsedSeconds * mouseComp.DecelerationChangeSpeed
	} else {
		carComp.Deceleration -= elapsedSeconds * mouseComp.DecelerationChangeSpeed
	}
	carComp.Deceleration = dprec.Clamp(carComp.Deceleration, 0.0, 1.0)

	if s.mouseScroll < -0.1 {
		carComp.Gear = CarGearReverse
	}
	if s.mouseScroll > 0.1 {
		carComp.Gear = CarGearForward
	}
	s.mouseScroll = 0.0

	carComp.Recover = s.mouseButtonStates[ui.MouseButtonMiddle]

	chassis := carComp.Car.Chassis()
	position := chassis.Body().Position()

	camera := s.gfxScene.ActiveCamera()
	viewport := graphics.Viewport{
		Width:  s.mouseAreaWidth,
		Height: s.mouseAreaHeight,
	}
	start, end := s.gfxScene.Ray(viewport, camera, s.mouseX, s.mouseY)

	line := collision.NewLine(start, end)

	intersection, ok := collision.LineWithSurfaceIntersectionPoint(line, position, dprec.BasisYVec3())
	if !ok {
		sphere := collision.NewSphere(position, 1000.0)
		_, intersection, ok = collision.LineWithSphereIntersectionPoints(line, sphere)
	}
	if ok {
		delta := dprec.Vec3Diff(intersection, position)
		delta.Y = 0.0

		forward := chassis.Body().Orientation().OrientationZ()
		forward.Y = 0.0

		sin := dprec.Vec3Cross(
			dprec.UnitVec3(forward),
			dprec.UnitVec3(delta),
		)
		angle := dprec.Angle(dprec.Sign(sin.Y)) * dprec.Asin(sin.Length())

		maxSteeringAngle := carComp.Car.Axes()[0].MaxSteeringAngle()
		carComp.SteeringAmount = float64(dprec.Clamp(-dprec.Angle(angle), -maxSteeringAngle, maxSteeringAngle) / maxSteeringAngle)

	} else {
		carComp.SteeringAmount = 0.0
	}
}

func (s *CarSystem) updateGamepad(elapsedSeconds float64, entity *ecs.Entity) {
	var carComp *CarComponent
	ecs.FetchComponent(entity, &carComp)
	var gamepadComp *CarGamepadControl
	ecs.FetchComponent(entity, &gamepadComp)

	gamepad := gamepadComp.Gamepad

	if !gamepad.Connected() || !gamepad.Supported() {
		return
	}
	s.updateRPM(gamepad.RightTrigger(), elapsedSeconds)
	leftStickX := gamepad.LeftStickX()
	carComp.SteeringAmount = leftStickX // * leftStickX * leftStickX
	carComp.Acceleration = (s.rpm - idleRPM) / (maxRPM - idleRPM)
	carComp.Deceleration = gamepad.LeftTrigger()
	carComp.SideBrake = gamepad.LeftStickY()
	if gamepad.BackButton() {
		carComp.Gear = CarGearReverse
	}
	if gamepad.ForwardButton() {
		carComp.Gear = CarGearForward
	}
	carComp.Recover = gamepad.ActionUpButton()
	gamepad.Pulse(s.ffbForce, 0)
}

var cnt = 0

func (s *CarSystem) updateCar(elapsedSeconds float64, entity *ecs.Entity) {
	// TODO: Run this inside physics loop for smooth operation.

	var carComp *CarComponent
	ecs.FetchComponent(entity, &carComp)
	var (
		car         = carComp.Car
		chassisBody = car.Chassis().Body()
	)

	for _, light := range carComp.Car.Chassis().HeadLights() {
		light.SetActive(carComp.LightsOn)
	}
	for _, light := range carComp.Car.Chassis().BeamLights() {
		light.SetActive(carComp.LightsOn)
	}
	for _, light := range carComp.Car.Chassis().TailLights() {
		light.SetActive(carComp.LightsOn)
	}
	for _, light := range carComp.Car.Chassis().StopLights() {
		light.SetActive(carComp.Deceleration > 0.1)
	}

	if carComp.Recover {
		rotationVector := dprec.Vec3Cross(
			chassisBody.Orientation().OrientationY(),
			dprec.BasisYVec3(),
		)
		chassisBody.SetAngularVelocity(dprec.Vec3Prod(
			rotationVector, 100*elapsedSeconds,
		))
		velocity := chassisBody.Velocity()
		velocity.Y = 2.0
		chassisBody.SetVelocity(velocity)
	}
	cnt++
	angle := dprec.Angle(carComp.SteeringAmount)
	defer func() {
		s.lastSteerAngle = angle
	}()
	for idx, axis := range car.Axes() {
		// TODO: Use Ackermann steering. Needs an additional steering offset (intersection line) parameter.
		steeringAngle := -axis.maxSteeringAngle * angle
		steeringQuat := dprec.RotationQuat(steeringAngle, dprec.BasisYVec3())
		direction := dprec.QuatVec3Rotation(steeringQuat, dprec.BasisXVec3())

		leftDirectionSolver := axis.leftWheel.directionSolver
		leftDirectionSolver.SetPrimaryDirection(direction)

		rightDirectionSolver := axis.rightWheel.directionSolver
		rightDirectionSolver.SetPrimaryDirection(direction)

		// Acceleration
		var deltaVelocity float64
		if carComp.Gear == CarGearForward {
			deltaVelocity = axis.maxAcceleration * carComp.Acceleration * elapsedSeconds
		} else {
			deltaVelocity = -axis.maxAcceleration * carComp.Acceleration * axis.reverseRatio * elapsedSeconds
		}

		leftWheelBody := axis.LeftWheel().Body()
		rightWheelBody := axis.RightWheel().Body()

		leftWheelBody.SetAngularVelocity(dprec.Vec3Sum(leftWheelBody.AngularVelocity(),
			dprec.Vec3Prod(leftWheelBody.Orientation().OrientationX(), deltaVelocity-leftWheelBody.Velocity().Z*0.01),
		))
		rightWheelBody.SetAngularVelocity(dprec.Vec3Sum(rightWheelBody.AngularVelocity(),
			dprec.Vec3Prod(rightWheelBody.Orientation().OrientationX(), deltaVelocity-rightWheelBody.Velocity().Z*0.01),
		))

		// Braking
		if carComp.Deceleration > 0.0 {
			// TODO: Implement ABS

			leftWheelVelocity := dprec.Vec3Dot(
				leftWheelBody.AngularVelocity(),
				leftWheelBody.Orientation().OrientationX(),
			)
			leftWheelCorrection := -dprec.Min(axis.maxBraking*carComp.Deceleration*elapsedSeconds, leftWheelVelocity)
			leftWheelBody.SetAngularVelocity(dprec.Vec3Sum(
				leftWheelBody.AngularVelocity(),
				dprec.Vec3Prod(leftWheelBody.Orientation().OrientationX(), leftWheelCorrection),
			))

			rightWheelVelocity := dprec.Vec3Dot(
				rightWheelBody.AngularVelocity(),
				rightWheelBody.Orientation().OrientationX(),
			)
			rightWheelCorrection := -dprec.Min(axis.maxBraking*carComp.Deceleration*elapsedSeconds, rightWheelVelocity)
			rightWheelBody.SetAngularVelocity(dprec.Vec3Sum(
				rightWheelBody.AngularVelocity(),
				dprec.Vec3Prod(rightWheelBody.Orientation().OrientationX(), rightWheelCorrection),
			))
		}
		// Side Braking
		if idx == 1 {
			if carComp.SideBrake > 0.0 {
				leftWheelVelocity := dprec.Vec3Dot(
					leftWheelBody.AngularVelocity(),
					leftWheelBody.Orientation().OrientationX(),
				)
				leftWheelCorrection := -dprec.Min(axis.maxBraking*carComp.SideBrake*elapsedSeconds, leftWheelVelocity)
				leftWheelBody.SetAngularVelocity(dprec.Vec3Sum(
					leftWheelBody.AngularVelocity(),
					dprec.Vec3Prod(leftWheelBody.Orientation().OrientationX(), leftWheelCorrection),
				))

				rightWheelVelocity := dprec.Vec3Dot(
					rightWheelBody.AngularVelocity(),
					rightWheelBody.Orientation().OrientationX(),
				)
				rightWheelCorrection := -dprec.Min(axis.maxBraking*carComp.SideBrake*elapsedSeconds, rightWheelVelocity)
				rightWheelBody.SetAngularVelocity(dprec.Vec3Sum(
					rightWheelBody.AngularVelocity(),
					dprec.Vec3Prod(rightWheelBody.Orientation().OrientationX(), rightWheelCorrection),
				))
			}
		}
		if idx == 0 {
			orientation := chassisBody.Orientation()
			w := s.carDefinition.axesDef[idx].width
			offsetL := s.carDefinition.axesDef[idx].position
			base := dprec.Vec3MultiSum(
				chassisBody.Position(),
				dprec.Vec3Prod(orientation.OrientationY(), offsetL.Y),
				dprec.Vec3Prod(orientation.OrientationZ(), offsetL.Z),
			)
			l := 1 - dprec.Vec3Dot(
				dprec.Vec3Diff(
					dprec.Vec3Sum(base, dprec.Vec3Prod(orientation.OrientationX(), offsetL.X+w/2)),
					leftWheelBody.Position(),
				),
				orientation.OrientationY(),
			)/0.16
			r := 1 - dprec.Vec3Dot(
				dprec.Vec3Diff(
					dprec.Vec3Sum(base, dprec.Vec3Prod(orientation.OrientationX(), offsetL.X-w/2)),
					rightWheelBody.Position(),
				),
				orientation.OrientationY(),
			)/0.16
			latForce := CalculateLateralForceFromStrokeDiff(l, r)
			torque := -s.torque(CalculateSelfAligningTorque(latForce))
			s.ffbForce = dprec.Clamp(torque/20, -1, 1)
			load := (l + r) / 2
			s.ffbForce += 0.5 * load * (angle - s.lastSteerAngle).Radians() / elapsedSeconds
			freq := s.rpm / 60 / 4
			s.ffbTick += elapsedSeconds
			s.ffbForce += dprec.Clamp(
				0.1*(s.rpm/maxRPM)*math.Sin(2*math.Pi*freq*float64(s.ffbTick)),
				-0.1, 0.1,
			)
			if cnt%10 == 0 {
				//log.Info("f: %v, s: %v, t: %v", load, latForce, torque)
			}
		}
	}
}

func MovingAverage(windowSize int) func(float64) float64 {
	values := make([]float64, 0, windowSize)
	sum := 0.0

	return func(newVal float64) float64 {
		if len(values) < windowSize {
			values = append(values, newVal)
			sum += newVal
			if len(values) < windowSize {
				// まだ6点集まっていないので平均は計算せず0を返すなど適宜調整
				return 0
			}
			return sum / float64(windowSize)
		}
		// 古い値を引いて新しい値を足す
		sum -= values[0]
		values = values[1:]
		values = append(values, newVal)
		sum += newVal
		return sum / float64(windowSize)
	}
}

// サスペンション左右ストローク差から横力を算出する関数
// leftStroke, rightStroke: 左右サスの沈み量[m]
// rollStiffness: ロール剛性[N/m]
// tread: トレッド幅[m]
// cgHeight: 重心高[m]
func CalculateLateralForceFromStrokeDiff(
	leftStroke float64,
	rightStroke float64,
) float64 {
	const (
		rollStiffness = 100
		tread         = 1.5
		cgHeight      = 1.0
	)
	// ストローク差から荷重移動量を計算
	strokeDiff := rightStroke - leftStroke
	deltaWeight := strokeDiff * rollStiffness

	// 横力 F_y を算出（重心高とトレッド幅から静力学的に換算）
	// F_y = 荷重移動量 * トレッド / (2 * 重心高)
	lateralForce := deltaWeight * tread / (2.0 * cgHeight)

	return lateralForce
}

// CalculateSelfAligningTorque: セルフアライニングトルク計算関数
func CalculateSelfAligningTorque(lateralForce float64) float64 {
	const (
		casterAngle     = 15.0 * math.Pi / 180.0 // キャスター角 15度
		mechanicalTrail = 0.05                   // キャスタートレール[m]
		pneumaticTrail  = 0.1                    // ニューマチックトレール[m]
	)
	// セルフアライニングトルク T を計算
	trailSum := mechanicalTrail + pneumaticTrail
	torque := trailSum * math.Cos(casterAngle) * lateralForce
	return torque
}
